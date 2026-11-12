package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"ticket-system/internal/adapter/email"
	"ticket-system/internal/adapter/excel"
	"ticket-system/internal/adapter/httpapi"
	"ticket-system/internal/adapter/postgres"
	"ticket-system/internal/config"
	"ticket-system/internal/usecase"
)

func Run(cfg config.Config) error {
	location, err := time.LoadLocation(cfg.Timezone)
	if err != nil {
		log.Printf("could not load timezone %q; using UTC: %v", cfg.Timezone, err)
		location = time.UTC
	}

	employees, err := excel.NewEmployeeStore(cfg.ExcelPath, cfg.ExcelSheet, cfg.IncludeInactive)
	if err != nil {
		return fmt.Errorf("load employee workbook %q: %w", cfg.ExcelPath, err)
	}

	db, err := postgres.Open(cfg)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("get database connection pool: %w", err)
	}
	defer sqlDB.Close()

	ticketRepository := postgres.NewTicketRepository(db)
	if err := ticketRepository.Migrate(); err != nil {
		return fmt.Errorf("migrate ticket schema: %w", err)
	}

	mailer := email.NewSender(cfg)
	if mailer.Enabled() {
		log.Printf("SMTP enabled: %s:%d (from: %s)", cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPFrom)
	} else {
		log.Println("SMTP is not configured; completed tickets will be saved without email")
	}

	ticketService := usecase.NewTicketService(ticketRepository, employees, mailer, location)
	healthCheck := func(ctx context.Context) error {
		return sqlDB.PingContext(ctx)
	}
	server := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           httpapi.NewRouter(ticketService, healthCheck, cfg),
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	serverErrors := make(chan error, 1)
	go func() {
		log.Printf("HTTP server listening on http://localhost:%s", cfg.Port)
		serverErrors <- server.ListenAndServe()
	}()

	select {
	case err := <-serverErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("serve HTTP: %w", err)
		}
	case <-ctx.Done():
		log.Println("shutting down HTTP server")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("HTTP shutdown error: %v", err)
		}
		ticketService.WaitForEmails(shutdownCtx)
		cancel()
	}
	return nil
}
