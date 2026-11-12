package httpapi

import (
	"context"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"ticket-system/internal/config"
	"ticket-system/internal/domain"
	"ticket-system/internal/usecase"
)

type TicketUseCase interface {
	SearchEmployees(query string, limit int) []domain.Employee
	Create(ctx context.Context, input usecase.CreateTicketInput) (domain.Ticket, bool, error)
	Complete(ctx context.Context, key string) (usecase.CompleteTicketResult, error)
	Assign(ctx context.Context, key, assignee string) (domain.Ticket, error)
	Get(ctx context.Context, key string) (domain.Ticket, error)
	List(ctx context.Context, status string, limit int) ([]domain.Ticket, int64, error)
}

type ticketView struct {
	ID           uint            `json:"id"`
	TicketNumber string          `json:"ticket_number"`
	Employee     domain.Employee `json:"employee"`
	Problem      string          `json:"problem"`
	AssignedTo   string          `json:"assigned_to"`
	Status       string          `json:"status"`
	EmailQueued  bool            `json:"email_queued"`
	EmailSentAt  *time.Time      `json:"email_sent_at"`
	CreatedAt    time.Time       `json:"created_at"`
}

func toView(ticket domain.Ticket, emailQueued bool) ticketView {
	return ticketView{
		ID: ticket.ID, TicketNumber: ticket.TicketNumber, Employee: ticket.Employee(),
		Status: ticket.Status, Problem: ticket.Problem, AssignedTo: ticket.AssignedTo,
		EmailQueued: emailQueued, EmailSentAt: ticket.EmailSentAt,
		CreatedAt: ticket.CreatedAt,
	}
}

type Handler struct {
	tickets TicketUseCase
}

func NewRouter(tickets TicketUseCase, healthCheck func(context.Context) error, cfg config.Config) *gin.Engine {
	if cfg.GinMode != "" {
		gin.SetMode(cfg.GinMode)
	}
	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery())
	_ = router.SetTrustedProxies(nil)

	router.GET("/healthz", func(c *gin.Context) {
		if err := healthCheck(c.Request.Context()); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"ok": false, "database": "unavailable"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true, "database": "connected"})
	})

	handler := &Handler{tickets: tickets}
	site := router.Group("/")
	if cfg.AppUser != "" && cfg.AppPassword != "" {
		site.Use(gin.BasicAuth(gin.Accounts{cfg.AppUser: cfg.AppPassword}))
		log.Println("🔒 Basic Auth enabled")
	}
	site.StaticFile("/", "./static/index.html")
	site.StaticFile("/dashboard", "./static/index.html")
	site.StaticFile("/admin", "./static/admin.html")

	api := site.Group("/api")
	api.GET("/search", handler.searchEmployees)
	api.GET("/tickets", handler.listTickets)
	api.POST("/tickets", handler.createTicket)
	api.GET("/tickets/:id", handler.getTicket)
	api.POST("/tickets/:id/done", handler.markDone)
	api.PUT("/tickets/:id/assignee", handler.assignTicket)

	router.NoRoute(func(c *gin.Context) {
		c.JSON(http.StatusNotFound, gin.H{"error": "page not found"})
	})
	return router
}

func (h *Handler) searchEmployees(c *gin.Context) {
	query := c.Query("q")
	if runes := []rune(query); len(runes) > 100 {
		query = string(runes[:100])
	}
	c.JSON(http.StatusOK, gin.H{"matches": h.tickets.SearchEmployees(query, usecase.MaxSearchResults)})
}

type createTicketRequest struct {
	Name       string `json:"name"`
	EmployeeID string `json:"employee_id"`
	Status     string `json:"status"`
	Problem    string `json:"problem"`
	AssignedTo string `json:"assigned_to"`
}

type assignTicketRequest struct {
	Assignee string `json:"assignee"`
}

func (h *Handler) assignTicket(c *gin.Context) {
	var request assignTicketRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON request"})
		return
	}
	ticket, err := h.tickets.Assign(c.Request.Context(), c.Param("id"), request.Assignee)
	switch err {
	case nil:
		c.JSON(http.StatusOK, toView(ticket, false))
	case usecase.ErrAssigneeTooLong:
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case domain.ErrNotFound:
		c.JSON(http.StatusNotFound, gin.H{"error": "ticket not found"})
	default:
		log.Printf("ticket assignment failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not assign ticket"})
	}
}

func (h *Handler) createTicket(c *gin.Context) {
	var request createTicketRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON request"})
		return
	}
	ticket, emailQueued, err := h.tickets.Create(c.Request.Context(), usecase.CreateTicketInput{
		Name: request.Name, EmployeeID: request.EmployeeID, Status: request.Status,
		Problem: request.Problem, AssignedTo: request.AssignedTo,
	})
	if err != nil {
		h.writeCreateError(c, err)
		return
	}
	c.JSON(http.StatusCreated, toView(ticket, emailQueued))
}

func (h *Handler) writeCreateError(c *gin.Context, err error) {
	switch {
	case err == usecase.ErrInvalidStatus:
		c.JSON(http.StatusBadRequest, gin.H{"error": "status must be 'done' or 'pending'"})
	case err == usecase.ErrEmployeeMissing:
		c.JSON(http.StatusBadRequest, gin.H{"error": "employee name is required"})
	case err == usecase.ErrEmployeeUnknown:
		c.JSON(http.StatusNotFound, gin.H{"error": "employee not found"})
	case err == usecase.ErrProblemRequired, err == usecase.ErrProblemTooLong, err == usecase.ErrAssigneeTooLong:
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	default:
		if ambiguous, ok := err.(*usecase.AmbiguousEmployeeError); ok {
			c.JSON(http.StatusConflict, gin.H{"error": "multiple employees match", "matches": ambiguous.Matches})
			return
		}
		log.Printf("ticket creation failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not save ticket"})
	}
}

func (h *Handler) markDone(c *gin.Context) {
	result, err := h.tickets.Complete(c.Request.Context(), c.Param("id"))
	if err != nil {
		if err == domain.ErrNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "ticket not found"})
			return
		}
		log.Printf("ticket completion failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not update ticket"})
		return
	}
	if result.AlreadyDone {
		c.JSON(http.StatusOK, gin.H{
			"message": "ticket was already completed", "already_done": true,
			"ticket": toView(result.Ticket, false),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"message": "ticket completed", "already_done": false,
		"ticket": toView(result.Ticket, result.EmailQueued),
	})
}

func (h *Handler) getTicket(c *gin.Context) {
	ticket, err := h.tickets.Get(c.Request.Context(), c.Param("id"))
	if err == domain.ErrNotFound {
		c.JSON(http.StatusNotFound, gin.H{"error": "ticket not found"})
		return
	}
	if err != nil {
		log.Printf("ticket lookup failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not find ticket"})
		return
	}
	c.JSON(http.StatusOK, toView(ticket, false))
}

func (h *Handler) listTickets(c *gin.Context) {
	limit, err := strconv.Atoi(c.DefaultQuery("limit", "100"))
	if err != nil || limit <= 0 || limit > usecase.MaxTicketLimit {
		limit = 100
	}
	status := strings.ToLower(strings.TrimSpace(c.Query("status")))
	tickets, total, err := h.tickets.List(c.Request.Context(), status, limit)
	if err != nil {
		if err == usecase.ErrInvalidFilter {
			c.JSON(http.StatusBadRequest, gin.H{"error": "status must be 'done' or 'pending'"})
			return
		}
		log.Printf("ticket list failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not load tickets"})
		return
	}
	views := make([]ticketView, 0, len(tickets))
	for _, ticket := range tickets {
		views = append(views, toView(ticket, false))
	}
	c.JSON(http.StatusOK, gin.H{"tickets": views, "count": len(views), "total": total})
}
