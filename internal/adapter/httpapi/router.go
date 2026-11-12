package httpapi

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	groqapi "ticket-system/internal/adapter/groq"
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
	tickets        TicketUseCase
	assistant      *groqapi.Client
	assistantMu    sync.Mutex
	assistantRates map[string]assistantRateEntry
}

type assistantRateEntry struct {
	window time.Time
	count  int
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

	handler := &Handler{tickets: tickets, assistant: groqapi.NewClient(cfg.GroqAPIKey, cfg.GroqModel), assistantRates: make(map[string]assistantRateEntry)}
	site := router.Group("/")
	if cfg.AppUser != "" && cfg.AppPassword != "" {
		site.Use(gin.BasicAuth(gin.Accounts{cfg.AppUser: cfg.AppPassword}))
		log.Println("🔒 Basic Auth enabled")
	}
	site.StaticFile("/", "./static/index.html")
	site.StaticFile("/dashboard", "./static/index.html")
	site.StaticFile("/admin", "./static/admin.html")
	site.StaticFile("/theme.css", "./static/theme.css")
	site.StaticFile("/theme.js", "./static/theme.js")

	api := site.Group("/api")
	api.GET("/search", handler.searchEmployees)
	api.GET("/assistant/status", handler.assistantStatus)
	api.POST("/assistant", handler.assistantChat)
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

func (h *Handler) assistantStatus(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"enabled": h.assistant.Enabled()})
}

type assistantChatRequest struct {
	Messages           []groqapi.Message `json:"messages"`
	SelectedEmployeeID string            `json:"selected_employee_id"`
	SelectedAssignee   string            `json:"selected_assignee"`
}

var assistantAssignees = []string{
	"Rupok Kumar Roy", "Towhedul Islam", "Md. Imam Hossan Chowdhury",
	"Muhammad Fokhrul Hasan", "Md. Tanvir Ahamed Sarkar", "Md. Fahad Hossen",
	"Mabubul Alam Patwary",
}

func resolveAssistantAssignee(value string) string {
	for _, name := range assistantAssignees {
		if normalizeAssistantValue(name) == normalizeAssistantValue(value) {
			return name
		}
	}
	return ""
}

func normalizeAssistantValue(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(value), " "))
}

func (h *Handler) assistantChat(c *gin.Context) {
	if !h.assistant.Enabled() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Groq API is not configured yet"})
		return
	}
	if !h.allowAssistantRequest(c.ClientIP(), time.Now()) {
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "অনেকগুলো request হয়েছে। এক মিনিট পরে আবার চেষ্টা করুন।"})
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 20<<10)
	var request assistantChatRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid assistant request"})
		return
	}
	if len(request.Messages) == 0 || len(request.Messages) > 12 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "send between 1 and 12 messages"})
		return
	}
	contentLength := 0
	for i, message := range request.Messages {
		message.Role = strings.ToLower(strings.TrimSpace(message.Role))
		if message.Role != "user" && message.Role != "assistant" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid conversation role"})
			return
		}
		message.Content = strings.TrimSpace(message.Content)
		if message.Content == "" || len([]rune(message.Content)) > 1200 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "each message must contain 1 to 1200 characters"})
			return
		}
		contentLength += len(message.Content)
		request.Messages[i] = message
	}
	if contentLength > 6000 || request.Messages[len(request.Messages)-1].Role != "user" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "conversation is too long or does not end with a user message"})
		return
	}
	details, err := h.assistant.ExtractTicket(c.Request.Context(), request.Messages)
	if err != nil {
		log.Printf("assistant extraction failed: %v", err)
		c.JSON(http.StatusBadGateway, gin.H{"error": "AI service could not process the request. Check Groq API settings and try again."})
		return
	}
	details.EmployeeName = strings.TrimSpace(details.EmployeeName)
	details.Problem = strings.TrimSpace(details.Problem)
	details.Assignee = strings.TrimSpace(details.Assignee)
	if len([]rune(details.Problem)) > 2000 {
		c.JSON(http.StatusOK, gin.H{"reply": "সমস্যার বিবরণ ২০০০ অক্ষরের মধ্যে ছোট করে লিখুন।"})
		return
	}
	if len([]rune(details.Assignee)) > 100 {
		c.JSON(http.StatusOK, gin.H{"reply": "Assignee-এর নাম ১০০ অক্ষরের মধ্যে লিখুন।"})
		return
	}
	selectedAssignee := strings.TrimSpace(request.SelectedAssignee)
	if selectedAssignee != "" {
		details.Assignee = resolveAssistantAssignee(selectedAssignee)
		if details.Assignee == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "selected assignee is invalid; choose an IT Team member"})
			return
		}
	} else if details.Assignee != "" {
		details.Assignee = resolveAssistantAssignee(details.Assignee)
	}
	if details.Problem == "" {
		reply := "সমস্যাটি কী হচ্ছে, একটু বিস্তারিত লিখুন।"
		if details.EmployeeName == "" {
			reply = "কোন কর্মীর জন্য ticket করবেন এবং কী সমস্যা হচ্ছে? কর্মীর পূর্ণ নামসহ লিখুন।"
		}
		c.JSON(http.StatusOK, gin.H{"reply": reply})
		return
	}
	var employee domain.Employee
	if employeeID := strings.TrimSpace(request.SelectedEmployeeID); employeeID != "" {
		matches := h.tickets.SearchEmployees(employeeID, usecase.MaxSearchResults)
		for _, match := range matches {
			if strings.EqualFold(strings.TrimSpace(match.EmployeeID), employeeID) {
				employee = match
				break
			}
		}
		if employee.EmployeeID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "selected employee is invalid; choose again"})
			return
		}
	} else {
		if details.EmployeeName == "" {
			c.JSON(http.StatusOK, gin.H{"reply": "কোন কর্মীর জন্য ticket করবেন? কর্মীর পূর্ণ নাম লিখুন।"})
			return
		}
		matches := h.tickets.SearchEmployees(details.EmployeeName, usecase.MaxSearchResults)
		exact := make([]domain.Employee, 0, len(matches))
		for _, match := range matches {
			if normalizeAssistantValue(match.Name) == normalizeAssistantValue(details.EmployeeName) {
				exact = append(exact, match)
			}
		}
		if len(exact) != 1 {
			if len(matches) == 0 {
				c.JSON(http.StatusOK, gin.H{"reply": fmt.Sprintf("“%s” নামে কর্মী খুঁজে পাইনি। পূর্ণ নাম বা employee ID লিখুন।", details.EmployeeName)})
				return
			}
			options := make([]gin.H, 0, len(matches))
			for _, match := range matches {
				options = append(options, gin.H{"name": match.Name, "employee_id": match.EmployeeID, "department": match.Department})
			}
			c.JSON(http.StatusOK, gin.H{"reply": "সঠিক কর্মীটি বেছে নিন, তারপর ticket তৈরি হবে।", "matches": options})
			return
		}
		employee = exact[0]
	}
	if details.Problem == "" {
		c.JSON(http.StatusOK, gin.H{"reply": "কী সমস্যা হচ্ছে, একটু বিস্তারিত লিখবেন?"})
		return
	}
	if details.Assignee == "" {
		c.JSON(http.StatusOK, gin.H{"reply": "সমস্যাটি বুঝেছি। কোন IT Team member-এর কাছে assign করব? নিচের তালিকা থেকে একজনকে বেছে নিন।", "needs_assignee": true})
		return
	}
	ticket, emailQueued, err := h.tickets.Create(c.Request.Context(), usecase.CreateTicketInput{
		Name: employee.Name, EmployeeID: employee.EmployeeID, Status: domain.StatusPending,
		Problem: details.Problem, AssignedTo: details.Assignee,
	})
	if err != nil {
		h.writeCreateError(c, err)
		return
	}
	reply := fmt.Sprintf("Pending ticket %s তৈরি হয়েছে।", ticket.TicketNumber)
	if ticket.AssignedTo != "" {
		reply += " Assignee: " + ticket.AssignedTo + "."
	}
	c.JSON(http.StatusCreated, gin.H{"reply": reply, "ticket": toView(ticket, emailQueued), "created": true})
}

func (h *Handler) allowAssistantRequest(clientIP string, now time.Time) bool {
	h.assistantMu.Lock()
	defer h.assistantMu.Unlock()
	if h.assistantRates == nil {
		h.assistantRates = make(map[string]assistantRateEntry)
	}
	for ip, entry := range h.assistantRates {
		if now.Sub(entry.window) > 2*time.Minute {
			delete(h.assistantRates, ip)
		}
	}
	entry := h.assistantRates[clientIP]
	if now.Sub(entry.window) >= time.Minute || entry.window.IsZero() {
		entry = assistantRateEntry{window: now}
	}
	if entry.count >= 15 {
		return false
	}
	entry.count++
	h.assistantRates[clientIP] = entry
	return true
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
