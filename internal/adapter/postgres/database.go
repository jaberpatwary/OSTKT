package postgres

import (
	"log"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"ticket-system/internal/config"
)

func Open(cfg config.Config) (*gorm.DB, error) {
	gormConfig := &gorm.Config{
		TranslateError: true,
		Logger: logger.New(log.New(os.Stdout, "\r\n", log.LstdFlags), logger.Config{
			SlowThreshold:             time.Second,
			LogLevel:                  logger.Warn,
			IgnoreRecordNotFoundError: true,
		}),
	}
	if dsn := strings.TrimSpace(cfg.DatabaseURL); dsn != "" {
		db, err := gorm.Open(postgres.New(postgres.Config{
			DSN:                  sanitizeDSN(dsn),
			PreferSimpleProtocol: true,
		}), gormConfig)
		if err != nil {
			return nil, err
		}
		sqlDB, err := db.DB()
		if err != nil {
			return nil, err
		}
		sqlDB.SetMaxOpenConns(5)
		sqlDB.SetMaxIdleConns(2)
		sqlDB.SetConnMaxIdleTime(4 * time.Minute)
		sqlDB.SetConnMaxLifetime(30 * time.Minute)
		log.Println("🐘 PostgreSQL connected")
		return db, nil
	}

	db, err := gorm.Open(sqlite.Open(cfg.SQLitePath+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"), gormConfig)
	if err != nil {
		return nil, err
	}
	if sqlDB, err := db.DB(); err == nil {
		sqlDB.SetMaxOpenConns(1)
	}
	log.Printf("🗂️ SQLite database: %s", cfg.SQLitePath)
	return db, nil
}

func sanitizeDSN(dsn string) string {
	parsed, err := url.Parse(dsn)
	if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") {
		return dsn
	}
	query := parsed.Query()
	query.Del("channel_binding")
	parsed.RawQuery = query.Encode()
	return parsed.String()
}
