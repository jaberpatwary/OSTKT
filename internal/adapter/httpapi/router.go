package httpapi

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"mime/multipart"
	"net/http"
	"net/mail"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/gin-gonic/gin"

	groqapi "ticket-system/internal/adapter/groq"
	"ticket-system/internal/config"
	"ticket-system/internal/domain"
	"ticket-system/internal/usecase"
)

type TicketUseCase interface {
	SearchEmployees(query string, limit int) []domain.Employee
	ListEmployees() []domain.Employee
	Create(ctx context.Context, input usecase.CreateTicketInput) (domain.Ticket, bool, error)
	Complete(ctx context.Context, key string) (usecase.CompleteTicketResult, error)
	SetStatus(ctx context.Context, key, status string) (domain.Ticket, bool, error)
	Assign(ctx context.Context, key, assignee string) (domain.Ticket, error)
	SetPriority(ctx context.Context, key, priority string) (domain.Ticket, error)
	SetSLAPaused(ctx context.Context, key string, paused bool) (domain.Ticket, error)
	Get(ctx context.Context, key string) (domain.Ticket, error)
	List(ctx context.Context, status string, from, until time.Time, limit int) ([]domain.Ticket, int64, error)
	Page(ctx context.Context, status string, from, until time.Time, query string, limit, offset int) ([]domain.Ticket, int64, error)
}

type TaskUseCase interface {
	Create(ctx context.Context, input usecase.CreateTaskInput) (domain.Task, error)
	List(ctx context.Context, filter domain.TaskFilter) ([]domain.Task, int64, error)
	Update(ctx context.Context, id uint, input usecase.UpdateTaskInput) (domain.Task, error)
}

type SecurityUseCase interface {
	Authenticate(ctx context.Context, username, password string) (domain.User, error)
	CreateSession(ctx context.Context, username, password, otp string) (domain.SessionCredentials, error)
	AuthenticateSession(ctx context.Context, token, csrf string) (domain.User, error)
	RevokeSession(ctx context.Context, token string) error
	MFAStatus(ctx context.Context, userID uint) (bool, error)
	BeginMFASetup(ctx context.Context, user domain.User) (string, string, error)
	ConfirmMFASetup(ctx context.Context, user domain.User, code string) error
	DisableMFA(ctx context.Context, user domain.User, password, code string) error
	ListUsers(ctx context.Context) ([]domain.User, error)
	CreateUser(ctx context.Context, input domain.UserInput) (domain.User, error)
	UpdateUser(ctx context.Context, id uint, input domain.UserUpdate) (domain.User, error)
	RecordAudit(ctx context.Context, event domain.AuditEvent) error
	ListAudit(ctx context.Context, limit int) ([]domain.AuditEvent, error)
}

type NotificationUseCase interface {
	List(ctx context.Context, userID uint, unreadOnly bool, limit int) ([]domain.Notification, int64, error)
	MarkRead(ctx context.Context, userID, notificationID uint) (bool, error)
	MarkAllRead(ctx context.Context, userID uint) error
}

type ActivityUseCase interface {
	Record(ctx context.Context, actor domain.User, workType string, workID uint, kind, message string) (domain.WorkActivity, error)
	List(ctx context.Context, workType string, workID uint, limit int) ([]domain.WorkActivity, error)
}

type AttachmentUseCase interface {
	Upload(ctx context.Context, actor domain.User, workType string, workID uint, filename string, source io.Reader) (domain.Attachment, error)
	List(ctx context.Context, workType string, workID uint) ([]domain.Attachment, error)
	Download(ctx context.Context, workType string, workID, attachmentID uint) (domain.Attachment, io.ReadCloser, error)
}

type PlatformUseCase interface {
	Services(ctx context.Context, includeInactive bool) ([]domain.ServiceCatalogItem, error)
	SaveService(ctx context.Context, item domain.ServiceCatalogItem) (domain.ServiceCatalogItem, error)
	Articles(ctx context.Context, search string, canSeeDrafts bool) ([]domain.KnowledgeArticle, error)
	SaveArticle(ctx context.Context, item domain.KnowledgeArticle) (domain.KnowledgeArticle, error)
	Assets(ctx context.Context, search string) ([]domain.Asset, error)
	SaveAsset(ctx context.Context, item domain.Asset) (domain.Asset, error)
	Changes(ctx context.Context, status string) ([]domain.ChangeRequest, error)
	SaveChange(ctx context.Context, item domain.ChangeRequest, creating bool) (domain.ChangeRequest, error)
	SetChangeStatus(ctx context.Context, id uint, status string) (domain.ChangeRequest, error)
	Incidents(ctx context.Context, recordType, status string) ([]domain.IncidentRecord, error)
	SaveIncident(ctx context.Context, item domain.IncidentRecord, creating bool) (domain.IncidentRecord, error)
	SetIncidentStatus(ctx context.Context, id uint, status string) (domain.IncidentRecord, error)
	ClaimInboundEmail(ctx context.Context, messageID string) (bool, error)
	InboundEmailStatus(ctx context.Context, messageID string) (string, bool, error)
	CompleteInboundEmail(ctx context.Context, messageID, ticketNumber string) error
	ReleaseInboundEmail(ctx context.Context, messageID string) error
}

type ticketView struct {
	ID           uint            `json:"id"`
	TicketNumber string          `json:"ticket_number"`
	Employee     domain.Employee `json:"employee"`
	Problem      string          `json:"problem"`
	AssetTag     string          `json:"asset_tag"`
	CreatedBy    string          `json:"created_by"`
	AssignedTo   string          `json:"assigned_to"`
	Priority     string          `json:"priority"`
	SLADueAt     time.Time       `json:"sla_due_at"`
	SLAPaused    bool            `json:"sla_paused"`
	Status       string          `json:"status"`
	EmailQueued  bool            `json:"email_queued"`
	EmailSentAt  *time.Time      `json:"email_sent_at"`
	CreatedAt    time.Time       `json:"created_at"`
}

type taskView struct {
	ID          uint      `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	CreatedBy   string    `json:"created_by"`
	AssignedTo  string    `json:"assigned_to"`
	Priority    string    `json:"priority"`
	DueDate     string    `json:"due_date"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func toTaskView(task domain.Task) taskView {
	createdBy := strings.TrimSpace(task.CreatedBy)
	if createdBy == "" {
		createdBy = usecase.DefaultTaskCreator
	}
	assignedTo := strings.TrimSpace(task.AssignedTo)
	if assignedTo == "" {
		assignedTo = usecase.DefaultTaskAssignee
	}
	return taskView{ID: task.ID, Title: task.Title, Description: task.Description, CreatedBy: createdBy, AssignedTo: assignedTo, Priority: task.Priority, DueDate: task.DueDate, Status: task.Status, CreatedAt: task.CreatedAt, UpdatedAt: task.UpdatedAt}
}

func toView(ticket domain.Ticket, emailQueued bool) ticketView {
	return ticketView{
		ID: ticket.ID, TicketNumber: ticket.TicketNumber, Employee: ticket.Employee(),
		Status: ticket.Status, Problem: ticket.Problem, AssetTag: ticket.AssetTag, CreatedBy: ticket.CreatedBy, AssignedTo: ticket.AssignedTo,
		Priority: ticket.Priority, SLADueAt: ticket.SLADueAt, SLAPaused: ticket.SLAPausedAt != nil,
		EmailQueued: emailQueued, EmailSentAt: ticket.EmailSentAt,
		CreatedAt: ticket.CreatedAt,
	}
}

type Handler struct {
	tickets             TicketUseCase
	tasks               TaskUseCase
	security            SecurityUseCase
	notifications       NotificationUseCase
	activities          ActivityUseCase
	attachments         AttachmentUseCase
	platform            PlatformUseCase
	emailWebhookSecret  string
	sessionCookieSecure bool
	loginMu             sync.Mutex
	loginFailures       map[string]loginRateEntry
	assistant           *groqapi.Client
	assistantMu         sync.Mutex
	assistantRates      map[string]assistantRateEntry
}

type assistantRateEntry struct {
	window time.Time
	count  int
}

type loginRateEntry struct {
	window time.Time
	count  int
}

var assistantGluedAssign = regexp.MustCompile(`(?i)\bassing([[:alpha:]])`)
var assistantMisspelledAssign = regexp.MustCompile(`(?i)\bassing\b`)
var inboundTicketReference = regexp.MustCompile(`(?i)\bTKT-\d{6}\b`)

func normalizeAssistantBanglish(text string) string {
	text = assistantGluedAssign.ReplaceAllString(text, "assign $1")
	return assistantMisspelledAssign.ReplaceAllString(text, "assign")
}

func NewRouter(tickets TicketUseCase, healthCheck func(context.Context) error, cfg config.Config, taskServices ...TaskUseCase) *gin.Engine {
	var tasks TaskUseCase
	if len(taskServices) > 0 {
		tasks = taskServices[0]
	}
	return newRouter(tickets, healthCheck, cfg, tasks, nil, nil, nil, nil)
}

func NewRouterWithSecurity(tickets TicketUseCase, healthCheck func(context.Context) error, cfg config.Config, tasks TaskUseCase, security SecurityUseCase) *gin.Engine {
	return newRouter(tickets, healthCheck, cfg, tasks, security, nil, nil, nil)
}

func NewRouterWithOperations(tickets TicketUseCase, healthCheck func(context.Context) error, cfg config.Config, tasks TaskUseCase, security SecurityUseCase, notifications NotificationUseCase) *gin.Engine {
	return newRouter(tickets, healthCheck, cfg, tasks, security, notifications, nil, nil)
}

func NewRouterWithActivity(tickets TicketUseCase, healthCheck func(context.Context) error, cfg config.Config, tasks TaskUseCase, security SecurityUseCase, notifications NotificationUseCase, activities ActivityUseCase) *gin.Engine {
	return newRouter(tickets, healthCheck, cfg, tasks, security, notifications, activities, nil)
}

func NewRouterWithAttachments(tickets TicketUseCase, healthCheck func(context.Context) error, cfg config.Config, tasks TaskUseCase, security SecurityUseCase, notifications NotificationUseCase, activities ActivityUseCase, attachments AttachmentUseCase, platformServices ...PlatformUseCase) *gin.Engine {
	return newRouter(tickets, healthCheck, cfg, tasks, security, notifications, activities, attachments, platformServices...)
}

func newRouter(tickets TicketUseCase, healthCheck func(context.Context) error, cfg config.Config, tasks TaskUseCase, security SecurityUseCase, notifications NotificationUseCase, activities ActivityUseCase, attachments AttachmentUseCase, platformServices ...PlatformUseCase) *gin.Engine {
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

	var platform PlatformUseCase
	if len(platformServices) > 0 {
		platform = platformServices[0]
	}
	handler := &Handler{tickets: tickets, tasks: tasks, security: security, notifications: notifications, activities: activities, attachments: attachments, platform: platform, emailWebhookSecret: cfg.EmailWebhookSecret, sessionCookieSecure: cfg.SessionCookieSecure, assistant: groqapi.NewClient(cfg.GroqAPIKey, cfg.GroqModel), assistantRates: make(map[string]assistantRateEntry), loginFailures: make(map[string]loginRateEntry)}
	router.POST("/integrations/email/inbound", handler.inboundEmail)
	site := router.Group("/")
	if security != nil {
		site.Use(handler.sessionAuthentication())
		site.GET("/login", func(c *gin.Context) {
			c.Header("Cache-Control", "no-store, no-cache, must-revalidate")
			c.File("./static/login.html")
		})
		site.GET("/account/security", func(c *gin.Context) { c.File("./static/account-security.html") })
		site.POST("/api/auth/login", handler.login)
		log.Println("Database-backed cookie session authentication enabled")
	} else if cfg.AppUser != "" && cfg.AppPassword != "" {
		site.Use(gin.BasicAuth(gin.Accounts{cfg.AppUser: cfg.AppPassword}))
		log.Println("🔒 Basic Auth enabled")
	}
	site.Use(func(c *gin.Context) {
		path := c.Request.URL.Path
		if path == "/" || path == "/dashboard" || path == "/workspace" || path == "/login" || path == "/account/security" || path == "/admin" || path == "/admin/tasks" || path == "/admin/users" || path == "/admin/audit" {
			c.Header("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
			c.Header("Pragma", "no-cache")
		}
		c.Next()
	})
	site.StaticFile("/", "./static/index.html")
	site.StaticFile("/dashboard", "./static/index.html")
	site.StaticFile("/workspace", "./static/operations.html")
	site.StaticFile("/admin", "./static/admin.html")
	site.StaticFile("/admin/tasks", "./static/admin-tasks.html")
	site.GET("/admin/users", handler.requireRoles("admin"), func(c *gin.Context) { c.File("./static/admin-users.html") })
	site.GET("/admin/audit", handler.requireRoles("admin", "manager"), func(c *gin.Context) { c.File("./static/admin-audit.html") })
	site.StaticFile("/theme.css", "./static/theme.css")
	site.StaticFile("/premium.css", "./static/premium.css")
	site.StaticFile("/experience.css", "./static/experience.css")
	site.StaticFile("/theme.js", "./static/theme.js")
	site.StaticFile("/notifications.js", "./static/notifications.js")
	site.StaticFile("/work-activity.js", "./static/work-activity.js")

	api := site.Group("/api")
	if security != nil {
		api.Use(handler.requireRoles("admin", "manager", "agent"))
		api.Use(handler.auditWrites())
		api.GET("/auth/me", handler.currentUser)
		api.POST("/auth/logout", handler.logout)
		api.GET("/auth/mfa", handler.mfaStatus)
		api.POST("/auth/mfa/setup", handler.beginMFASetup)
		api.POST("/auth/mfa/confirm", handler.confirmMFASetup)
		api.POST("/auth/mfa/disable", handler.disableMFA)
		api.GET("/admin/users", handler.requireRoles("admin"), handler.listUsers)
		api.POST("/admin/users", handler.requireRoles("admin"), handler.createUser)
		api.PUT("/admin/users/:id", handler.requireRoles("admin"), handler.updateUser)
		api.GET("/admin/audit", handler.requireRoles("admin", "manager"), handler.listAudit)
		if notifications != nil {
			api.GET("/notifications", handler.listNotifications)
			api.POST("/notifications/read-all", handler.markAllNotificationsRead)
			api.POST("/notifications/:id/read", handler.markNotificationRead)
		}
		if activities != nil {
			api.GET("/work/:type/:id/activity", handler.listWorkActivity)
			api.POST("/work/:type/:id/comments", handler.createWorkComment)
		}
		if attachments != nil {
			api.GET("/work/:type/:id/attachments", handler.listWorkAttachments)
			api.POST("/work/:type/:id/attachments", handler.uploadWorkAttachment)
			api.GET("/work/:type/:id/attachments/:attachmentID", handler.downloadWorkAttachment)
		}
	}
	if security == nil {
		api.GET("/auth/me", handler.currentUser)
	}
	api.GET("/search", handler.searchEmployees)
	api.GET("/assistant/status", handler.assistantStatus)
	api.POST("/assistant", handler.assistantChat)
	api.POST("/assistant/tasks", handler.assistantTaskChat)
	api.POST("/assistant/transcribe", handler.assistantTranscribe)
	api.GET("/tickets", handler.listTickets)
	api.GET("/tickets/export.csv", handler.exportTicketsCSV)
	api.POST("/tickets", handler.createTicket)
	api.GET("/tickets/:id", handler.getTicket)
	api.POST("/tickets/:id/done", handler.markDone)
	api.PATCH("/tickets/:id/status", handler.setTicketStatus)
	api.PUT("/tickets/:id/assignee", handler.assignTicket)
	api.PATCH("/tickets/:id/priority", handler.setTicketPriority)
	api.PATCH("/tickets/:id/sla", handler.setTicketSLAPause)
	api.GET("/tasks", handler.listTasks)
	api.GET("/tasks/export.csv", handler.exportTasksCSV)
	api.POST("/tasks", handler.createTask)
	api.PATCH("/tasks/:id", handler.updateTask)
	if platform != nil {
		api.GET("/platform/services", handler.listServices)
		api.GET("/platform/knowledge", handler.listKnowledge)
		api.GET("/platform/assets", handler.listAssets)
		api.GET("/platform/changes", handler.listChanges)
		api.GET("/platform/incidents", handler.listIncidents)
		if security != nil {
			api.POST("/platform/services", handler.requireRoles("admin", "manager"), handler.saveService)
			api.PUT("/platform/services/:id", handler.requireRoles("admin", "manager"), handler.saveService)
			api.POST("/platform/knowledge", handler.requireRoles("admin", "manager"), handler.saveKnowledge)
			api.PUT("/platform/knowledge/:id", handler.requireRoles("admin", "manager"), handler.saveKnowledge)
			api.POST("/platform/assets", handler.requireRoles("admin", "manager"), handler.saveAsset)
			api.PUT("/platform/assets/:id", handler.requireRoles("admin", "manager"), handler.saveAsset)
			api.POST("/platform/changes", handler.saveChange)
			api.PATCH("/platform/changes/:id/status", handler.requireRoles("admin", "manager"), handler.updateChangeStatus)
			api.POST("/platform/incidents", handler.saveIncident)
			api.PATCH("/platform/incidents/:id/status", handler.requireRoles("admin", "manager"), handler.updateIncidentStatus)
		} else {
			api.POST("/platform/services", handler.saveService)
			api.PUT("/platform/services/:id", handler.saveService)
			api.POST("/platform/knowledge", handler.saveKnowledge)
			api.PUT("/platform/knowledge/:id", handler.saveKnowledge)
			api.POST("/platform/assets", handler.saveAsset)
			api.PUT("/platform/assets/:id", handler.saveAsset)
			api.POST("/platform/changes", handler.saveChange)
			api.PATCH("/platform/changes/:id/status", handler.updateChangeStatus)
			api.POST("/platform/incidents", handler.saveIncident)
			api.PATCH("/platform/incidents/:id/status", handler.updateIncidentStatus)
		}
	}

	router.NoRoute(func(c *gin.Context) {
		c.JSON(http.StatusNotFound, gin.H{"error": "page not found"})
	})
	return router
}

type inboundEmailRequest struct {
	MessageID string `json:"message_id"`
	From      string `json:"from"`
	Subject   string `json:"subject"`
	Text      string `json:"text"`
}

// inboundEmail accepts a small provider-neutral JSON envelope. The upstream
// mail provider should sign: unix_timestamp + "." + raw_request_body using HMAC-SHA256.
func (h *Handler) inboundEmail(c *gin.Context) {
	if len(h.emailWebhookSecret) < 32 || h.platform == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "email integration is not configured"})
		return
	}
	timestamp := strings.TrimSpace(c.GetHeader("X-Email-Timestamp"))
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, (1<<20)+1))
	if err != nil || len(body) > 1<<20 {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "email payload is too large"})
		return
	}
	if !verifyEmailWebhookSignature(h.emailWebhookSecret, timestamp, c.GetHeader("X-Email-Signature"), body, time.Now()) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired email signature"})
		return
	}
	var message inboundEmailRequest
	if err := json.Unmarshal(body, &message); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid email payload"})
		return
	}
	message.MessageID = strings.TrimSpace(strings.Trim(message.MessageID, "<>"))
	message.Subject = strings.TrimSpace(message.Subject)
	message.Text = strings.TrimSpace(message.Text)
	if message.MessageID == "" || message.From == "" || (message.Subject == "" && message.Text == "") || len([]rune(message.Text)) > 30000 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "message_id, from, and email content are required"})
		return
	}
	address, err := mail.ParseAddress(message.From)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "from must contain a valid email address"})
		return
	}
	var employee *domain.Employee
	for _, candidate := range h.tickets.ListEmployees() {
		if strings.EqualFold(strings.TrimSpace(candidate.Email), address.Address) {
			if employee != nil && employee.EmployeeID != candidate.EmployeeID {
				c.JSON(http.StatusConflict, gin.H{"error": "sender email matches more than one employee"})
				return
			}
			match := candidate
			employee = &match
		}
	}
	if employee == nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "sender email is not in the employee directory"})
		return
	}
	threadNumber := strings.ToUpper(inboundTicketReference.FindString(message.Subject))
	var threadTicket domain.Ticket
	if threadNumber != "" {
		threadTicket, err = h.tickets.Get(c.Request.Context(), threadNumber)
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "ticket reference in email subject was not found"})
			} else {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "could not load referenced ticket"})
			}
			return
		}
		if !strings.EqualFold(strings.TrimSpace(threadTicket.Email), address.Address) {
			c.JSON(http.StatusForbidden, gin.H{"error": "sender does not match the requester for this ticket"})
			return
		}
		if h.activities == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "ticket reply threading is not available"})
			return
		}
	}
	if !h.claimInboundEmail(c, message.MessageID) {
		return
	}
	if threadNumber != "" {
		reply := strings.Join(nonEmptyParts(message.Subject, message.Text), "\n\n")
		if len([]rune(reply)) > 2000 {
			reply = string([]rune(reply)[:2000])
		}
		actor := domain.User{DisplayName: employee.Name, Role: "email"}
		if _, err := h.activities.Record(c.Request.Context(), actor, "ticket", threadTicket.ID, "comment", reply); err != nil {
			_ = h.platform.ReleaseInboundEmail(c.Request.Context(), message.MessageID)
			log.Printf("email reply history insert failed for %s: %v", threadNumber, err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not attach reply to ticket"})
			return
		}
		if err := h.platform.CompleteInboundEmail(c.Request.Context(), message.MessageID, threadTicket.TicketNumber); err != nil {
			log.Printf("email reply receipt finalization failed for %s: %v", threadNumber, err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "reply was saved but receipt could not be finalized"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true, "reply": true, "ticket_number": threadTicket.TicketNumber})
		return
	}
	problem := strings.Join(nonEmptyParts(message.Subject, message.Text), "\n\n")
	if len([]rune(problem)) > 2000 {
		problem = string([]rune(problem)[:2000])
	}
	ticket, _, err := h.tickets.Create(c.Request.Context(), usecase.CreateTicketInput{
		Name: employee.Name, EmployeeID: employee.EmployeeID, Status: domain.StatusPending,
		Problem: problem, CreatedBy: "Email integration",
	})
	if err != nil {
		_ = h.platform.ReleaseInboundEmail(c.Request.Context(), message.MessageID)
		log.Printf("email integration ticket creation failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not create ticket from email"})
		return
	}
	if err := h.platform.CompleteInboundEmail(c.Request.Context(), message.MessageID, ticket.TicketNumber); err != nil {
		log.Printf("email integration event completion failed for %s: %v", ticket.TicketNumber, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "ticket was created but email receipt could not be finalized"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"ok": true, "duplicate": false, "ticket": toView(ticket, false)})
}

func (h *Handler) claimInboundEmail(c *gin.Context, messageID string) bool {
	claimed, err := h.platform.ClaimInboundEmail(c.Request.Context(), messageID)
	if err != nil {
		log.Printf("email integration claim failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not accept email message"})
		return false
	}
	if !claimed {
		ticketNumber, completed, statusErr := h.platform.InboundEmailStatus(c.Request.Context(), messageID)
		if statusErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not check email delivery state"})
			return false
		}
		if !completed {
			c.Header("Retry-After", "60")
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "message is being processed; retry later"})
			return false
		}
		c.JSON(http.StatusOK, gin.H{"ok": true, "duplicate": true, "ticket_number": ticketNumber})
		return false
	}
	return true
}

func verifyEmailWebhookSignature(secret, timestamp, supplied string, body []byte, now time.Time) bool {
	if len(secret) < 32 {
		return false
	}
	unix, err := strconv.ParseInt(strings.TrimSpace(timestamp), 10, 64)
	if err != nil {
		return false
	}
	signedAt := time.Unix(unix, 0)
	if now.Sub(signedAt) > 5*time.Minute || signedAt.Sub(now) > 5*time.Minute {
		return false
	}
	supplied = strings.TrimPrefix(strings.TrimSpace(supplied), "sha256=")
	signature, err := hex.DecodeString(supplied)
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(strings.TrimSpace(timestamp) + "."))
	_, _ = mac.Write(body)
	return hmac.Equal(signature, mac.Sum(nil))
}

func nonEmptyParts(values ...string) []string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			parts = append(parts, value)
		}
	}
	return parts
}

func (h *Handler) listServices(c *gin.Context) {
	items, err := h.platform.Services(c.Request.Context(), isPlatformManager(c))
	if err != nil {
		platformFailure(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"services": items})
}

func (h *Handler) saveService(c *gin.Context) {
	var item domain.ServiceCatalogItem
	if !bindPlatformJSON(c, &item) {
		return
	}
	if c.Request.Method == http.MethodPost {
		item.ID = 0
	} else if !pathID(c, &item.ID) {
		return
	}
	saved, err := h.platform.SaveService(c.Request.Context(), item)
	if err != nil {
		platformFailure(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"service": saved})
}

func (h *Handler) listKnowledge(c *gin.Context) {
	items, err := h.platform.Articles(c.Request.Context(), c.Query("q"), isPlatformManager(c))
	if err != nil {
		platformFailure(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"articles": items})
}

func (h *Handler) saveKnowledge(c *gin.Context) {
	var item domain.KnowledgeArticle
	if !bindPlatformJSON(c, &item) {
		return
	}
	if c.Request.Method == http.MethodPost {
		item.ID = 0
	} else if !pathID(c, &item.ID) {
		return
	}
	saved, err := h.platform.SaveArticle(c.Request.Context(), item)
	if err != nil {
		platformFailure(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"article": saved})
}

func (h *Handler) listAssets(c *gin.Context) {
	items, err := h.platform.Assets(c.Request.Context(), c.Query("q"))
	if err != nil {
		platformFailure(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"assets": items})
}

func (h *Handler) saveAsset(c *gin.Context) {
	var item domain.Asset
	if !bindPlatformJSON(c, &item) {
		return
	}
	if c.Request.Method == http.MethodPost {
		item.ID = 0
	} else if !pathID(c, &item.ID) {
		return
	}
	saved, err := h.platform.SaveAsset(c.Request.Context(), item)
	if err != nil {
		platformFailure(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"asset": saved})
}

func (h *Handler) listChanges(c *gin.Context) {
	items, err := h.platform.Changes(c.Request.Context(), c.Query("status"))
	if err != nil {
		platformFailure(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"changes": items})
}

func (h *Handler) saveChange(c *gin.Context) {
	var item domain.ChangeRequest
	if !bindPlatformJSON(c, &item) {
		return
	}
	user, ok := c.Get(authenticatedUserKey)
	if creator, valid := user.(domain.User); ok && valid && strings.TrimSpace(item.RequestedBy) == "" {
		item.RequestedBy = creator.DisplayName
	}
	item.ID = 0
	saved, err := h.platform.SaveChange(c.Request.Context(), item, true)
	if err != nil {
		platformFailure(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"change": saved})
}

func (h *Handler) updateChangeStatus(c *gin.Context) {
	var payload struct {
		Status string `json:"status"`
	}
	if !bindPlatformJSON(c, &payload) {
		return
	}
	id, ok := parsePathID(c)
	if !ok {
		return
	}
	updated, err := h.platform.SetChangeStatus(c.Request.Context(), id, payload.Status)
	if err != nil {
		platformFailure(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"change": updated})
}

func (h *Handler) listIncidents(c *gin.Context) {
	items, err := h.platform.Incidents(c.Request.Context(), c.Query("type"), c.Query("status"))
	if err != nil {
		platformFailure(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"records": items})
}

func (h *Handler) saveIncident(c *gin.Context) {
	var item domain.IncidentRecord
	if !bindPlatformJSON(c, &item) {
		return
	}
	item.ID = 0
	if user, ok := c.Get(authenticatedUserKey); ok {
		if creator, valid := user.(domain.User); valid && strings.TrimSpace(item.Owner) == "" {
			item.Owner = creator.DisplayName
		}
	}
	saved, err := h.platform.SaveIncident(c.Request.Context(), item, true)
	if err != nil {
		platformFailure(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"record": saved})
}

func (h *Handler) updateIncidentStatus(c *gin.Context) {
	var payload struct {
		Status string `json:"status"`
	}
	if !bindPlatformJSON(c, &payload) {
		return
	}
	id, ok := parsePathID(c)
	if !ok {
		return
	}
	updated, err := h.platform.SetIncidentStatus(c.Request.Context(), id, payload.Status)
	if err != nil {
		platformFailure(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"record": updated})
}

func bindPlatformJSON(c *gin.Context, target any) bool {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 256*1024)
	if err := c.ShouldBindJSON(target); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON body"})
		return false
	}
	return true
}

func parsePathID(c *gin.Context) (uint, bool) {
	value, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || value == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid record id"})
		return 0, false
	}
	return uint(value), true
}

func pathID(c *gin.Context, target *uint) bool {
	id, ok := parsePathID(c)
	if ok {
		*target = id
	}
	return ok
}

func isPlatformManager(c *gin.Context) bool {
	value, ok := c.Get(authenticatedUserKey)
	if !ok {
		return false
	}
	user, ok := value.(domain.User)
	return ok && (user.Role == "admin" || user.Role == "manager")
}

func platformFailure(c *gin.Context, err error) {
	if errors.Is(err, usecase.ErrInvalidPlatformInput) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "check the required fields and allowed values"})
		return
	}
	if errors.Is(err, domain.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "record not found"})
		return
	}
	if errors.Is(err, domain.ErrInvalidTransition) {
		c.JSON(http.StatusConflict, gin.H{"error": "that status change is not allowed from the current state"})
		return
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "unique") || strings.Contains(message, "duplicate") {
		c.JSON(http.StatusConflict, gin.H{"error": "a record with that identifier already exists"})
		return
	}
	log.Printf("operations workspace request failed: %v", err)
	c.JSON(http.StatusInternalServerError, gin.H{"error": "operations workspace request failed"})
}

const authenticatedUserKey = "authenticatedUser"
const authenticatedSessionTokenKey = "authenticatedSessionToken"
const sessionCookieName = "ost_session"
const csrfCookieName = "ost_csrf"

func (h *Handler) authenticate() gin.HandlerFunc {
	return func(c *gin.Context) {
		username, password, ok := c.Request.BasicAuth()
		if !ok || len(username) > 256 || len(password) > 8<<10 {
			httpAuthChallenge(c)
			return
		}
		user, err := h.security.Authenticate(c.Request.Context(), username, password)
		if err != nil {
			httpAuthChallenge(c)
			return
		}
		c.Set(authenticatedUserKey, user)
		c.Next()
	}
}

func httpAuthChallenge(c *gin.Context) {
	c.Header("WWW-Authenticate", `Basic realm="Auto Ticket System", charset="UTF-8"`)
	c.AbortWithStatus(http.StatusUnauthorized)
}

func (h *Handler) sessionAuthentication() gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.Request.URL.Path
		if path == "/login" || path == "/api/auth/login" {
			c.Next()
			return
		}
		token, tokenErr := c.Cookie(sessionCookieName)
		csrf, csrfErr := c.Cookie(csrfCookieName)
		if tokenErr != nil || csrfErr != nil {
			h.unauthorized(c)
			return
		}
		user, err := h.security.AuthenticateSession(c.Request.Context(), token, csrf)
		if err != nil {
			h.clearSessionCookies(c)
			h.unauthorized(c)
			return
		}
		if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead && c.Request.Method != http.MethodOptions {
			header := c.GetHeader("X-CSRF-Token")
			if len(header) != len(csrf) || subtle.ConstantTimeCompare([]byte(header), []byte(csrf)) != 1 {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "request security token is missing or invalid; refresh the page and try again"})
				return
			}
		}
		c.Set(authenticatedUserKey, user)
		c.Set(authenticatedSessionTokenKey, token)
		c.Next()
	}
}

func (h *Handler) unauthorized(c *gin.Context) {
	if strings.HasPrefix(c.Request.URL.Path, "/api/") {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "login required"})
		return
	}
	c.Redirect(http.StatusSeeOther, "/login")
	c.Abort()
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	OTPCode  string `json:"otp_code"`
}

func (h *Handler) login(c *gin.Context) {
	clientIP := c.ClientIP()
	if !h.allowLoginAttempt(clientIP, time.Now()) {
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "too many login attempts; wait 15 minutes and try again"})
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10)
	var request loginRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "enter your username and password"})
		return
	}
	credentials, err := h.security.CreateSession(c.Request.Context(), request.Username, request.Password, request.OTPCode)
	if err != nil {
		if errors.Is(err, usecase.ErrInvalidCredentials) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "username or password is incorrect"})
			return
		}
		log.Printf("login session creation failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not sign in right now"})
		return
	}
	h.resetLoginAttempts(clientIP)
	maxAge := int(time.Until(credentials.ExpiresAt).Seconds())
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(sessionCookieName, credentials.Token, maxAge, "/", "", h.sessionCookieSecure, true)
	c.SetCookie(csrfCookieName, credentials.CSRFToken, maxAge, "/", "", h.sessionCookieSecure, false)
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{"user": credentials.User, "csrf_token": credentials.CSRFToken, "expires_at": credentials.ExpiresAt})
}

func (h *Handler) logout(c *gin.Context) {
	token, _ := c.Get(authenticatedSessionTokenKey)
	if raw, ok := token.(string); ok {
		if err := h.security.RevokeSession(c.Request.Context(), raw); err != nil {
			log.Printf("session revocation failed: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not sign out"})
			return
		}
	}
	h.clearSessionCookies(c)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) mfaStatus(c *gin.Context) {
	user, ok := currentAuthenticatedUser(c)
	if !ok {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	enabled, err := h.security.MFAStatus(c.Request.Context(), user.ID)
	if err != nil {
		log.Printf("MFA status lookup failed for user %d: %v", user.ID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not load MFA status"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"enabled": enabled})
}

func (h *Handler) beginMFASetup(c *gin.Context) {
	user, ok := currentAuthenticatedUser(c)
	if !ok {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	secret, uri, err := h.security.BeginMFASetup(c.Request.Context(), user)
	if err != nil {
		if errors.Is(err, usecase.ErrMFAAlreadyEnabled) {
			c.JSON(http.StatusConflict, gin.H{"error": "MFA is already enabled"})
			return
		}
		if errors.Is(err, usecase.ErrMFAUnavailable) {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "set MFA_ENCRYPTION_KEY before enrolling an authenticator"})
			return
		}
		log.Printf("MFA setup failed for user %d: %v", user.ID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not start MFA setup"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"secret": secret, "otpauth_uri": uri})
}

func (h *Handler) confirmMFASetup(c *gin.Context) {
	user, ok := currentAuthenticatedUser(c)
	if !ok {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	var request struct {
		Code string `json:"code"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "enter the six-digit authenticator code"})
		return
	}
	if err := h.security.ConfirmMFASetup(c.Request.Context(), user, request.Code); err != nil {
		if errors.Is(err, usecase.ErrMFAInvalidCode) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "authenticator code is incorrect or expired"})
			return
		}
		if errors.Is(err, usecase.ErrMFAAlreadyEnabled) {
			c.JSON(http.StatusConflict, gin.H{"error": "MFA is already enabled"})
			return
		}
		log.Printf("MFA confirmation failed for user %d: %v", user.ID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not enable MFA"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"enabled": true, "message": "MFA enabled. Sign in again using your authenticator code."})
}

func (h *Handler) disableMFA(c *gin.Context) {
	user, ok := currentAuthenticatedUser(c)
	if !ok {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	var request struct {
		Password string `json:"password"`
		Code     string `json:"code"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "enter your password and authenticator code"})
		return
	}
	if err := h.security.DisableMFA(c.Request.Context(), user, request.Password, request.Code); err != nil {
		if errors.Is(err, usecase.ErrInvalidCredentials) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "password or authenticator code is incorrect"})
			return
		}
		if errors.Is(err, usecase.ErrMFAInvalidCode) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "authenticator code is incorrect or expired"})
			return
		}
		log.Printf("MFA disable failed for user %d: %v", user.ID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not disable MFA"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"enabled": false, "message": "MFA disabled. All sessions have been signed out."})
}

func (h *Handler) clearSessionCookies(c *gin.Context) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(sessionCookieName, "", -1, "/", "", h.sessionCookieSecure, true)
	c.SetCookie(csrfCookieName, "", -1, "/", "", h.sessionCookieSecure, false)
}

func (h *Handler) allowLoginAttempt(ip string, now time.Time) bool {
	h.loginMu.Lock()
	defer h.loginMu.Unlock()
	if h.loginFailures == nil {
		h.loginFailures = make(map[string]loginRateEntry)
	}
	entry := h.loginFailures[ip]
	if now.Sub(entry.window) > 15*time.Minute || entry.window.IsZero() {
		entry = loginRateEntry{window: now}
	}
	if entry.count >= 20 {
		return false
	}
	entry.count++
	h.loginFailures[ip] = entry
	return true
}

func (h *Handler) resetLoginAttempts(ip string) {
	h.loginMu.Lock()
	delete(h.loginFailures, ip)
	h.loginMu.Unlock()
}

func (h *Handler) requireRoles(roles ...string) gin.HandlerFunc {
	allowed := make(map[string]struct{}, len(roles))
	for _, role := range roles {
		allowed[role] = struct{}{}
	}
	return func(c *gin.Context) {
		value, exists := c.Get(authenticatedUserKey)
		user, valid := value.(domain.User)
		if !exists || !valid {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}
		if _, ok := allowed[user.Role]; !ok {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "your role cannot access this resource"})
			return
		}
		c.Next()
	}
}

func (h *Handler) currentUser(c *gin.Context) {
	user, ok := currentAuthenticatedUser(c)
	if ok {
		c.JSON(http.StatusOK, gin.H{"user": user})
		return
	}
	username, _, basicAuth := c.Request.BasicAuth()
	if basicAuth && strings.TrimSpace(username) != "" {
		c.JSON(http.StatusOK, gin.H{"user": gin.H{"username": username, "display_name": username}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"user": gin.H{"username": "", "display_name": "Anonymous user"}})
}

func (h *Handler) listUsers(c *gin.Context) {
	users, err := h.security.ListUsers(c.Request.Context())
	if err != nil {
		log.Printf("user list failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not load users"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"users": users})
}

type createUserRequest struct {
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Role        string `json:"role"`
	Password    string `json:"password"`
}

func (h *Handler) createUser(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10)
	var request createUserRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user details"})
		return
	}
	user, err := h.security.CreateUser(c.Request.Context(), domain.UserInput{
		Username: request.Username, DisplayName: request.DisplayName,
		Role: request.Role, Password: request.Password, Active: true,
	})
	if err != nil {
		writeSecurityError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"user": user})
}

type updateUserRequest struct {
	DisplayName string `json:"display_name"`
	Role        string `json:"role"`
	Password    string `json:"password"`
	Active      bool   `json:"active"`
}

func (h *Handler) updateUser(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user ID"})
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10)
	var request updateUserRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user details"})
		return
	}
	user, err := h.security.UpdateUser(c.Request.Context(), uint(id), domain.UserUpdate{
		DisplayName: request.DisplayName, Role: request.Role,
		Password: request.Password, Active: request.Active,
	})
	if err != nil {
		writeSecurityError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"user": user})
}

func (h *Handler) listAudit(c *gin.Context) {
	limit, _ := strconv.Atoi(c.Query("limit"))
	events, err := h.security.ListAudit(c.Request.Context(), limit)
	if err != nil {
		log.Printf("audit list failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not load audit history"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"events": events})
}

func (h *Handler) listNotifications(c *gin.Context) {
	user, ok := currentAuthenticatedUser(c)
	if !ok {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	limit, _ := strconv.Atoi(c.Query("limit"))
	unreadOnly, _ := strconv.ParseBool(c.Query("unread_only"))
	items, unreadCount, err := h.notifications.List(c.Request.Context(), user.ID, unreadOnly, limit)
	if err != nil {
		log.Printf("notification list failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not load notifications"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"notifications": items, "unread_count": unreadCount})
}

func (h *Handler) markNotificationRead(c *gin.Context) {
	user, ok := currentAuthenticatedUser(c)
	if !ok {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	parsedID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || parsedID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid notification ID"})
		return
	}
	updated, err := h.notifications.MarkRead(c.Request.Context(), user.ID, uint(parsedID))
	if err != nil {
		log.Printf("notification mark-read failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not update notification"})
		return
	}
	if !updated {
		c.JSON(http.StatusNotFound, gin.H{"error": "notification not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"read": true})
}

func (h *Handler) markAllNotificationsRead(c *gin.Context) {
	user, ok := currentAuthenticatedUser(c)
	if !ok {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	if err := h.notifications.MarkAllRead(c.Request.Context(), user.ID); err != nil {
		log.Printf("mark all notifications read failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not update notifications"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"read": true})
}

type taskGetter interface {
	Get(ctx context.Context, id uint) (domain.Task, error)
}

func (h *Handler) resolveWorkID(c *gin.Context) (uint, bool) {
	workType := strings.ToLower(strings.TrimSpace(c.Param("type")))
	key := strings.TrimSpace(c.Param("id"))
	switch workType {
	case "ticket":
		ticket, err := h.tickets.Get(c.Request.Context(), key)
		if errors.Is(err, domain.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "ticket not found"})
			return 0, false
		}
		if err != nil {
			log.Printf("ticket activity lookup failed: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not find ticket"})
			return 0, false
		}
		return ticket.ID, true
	case "task":
		id, err := strconv.ParseUint(key, 10, 64)
		if err != nil || id == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid task ID"})
			return 0, false
		}
		getter, ok := h.tasks.(taskGetter)
		if !ok {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "task lookup is unavailable"})
			return 0, false
		}
		if _, err := getter.Get(c.Request.Context(), uint(id)); errors.Is(err, domain.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
			return 0, false
		} else if err != nil {
			log.Printf("task activity lookup failed: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not find task"})
			return 0, false
		}
		return uint(id), true
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "work type must be ticket or task"})
		return 0, false
	}
}

func (h *Handler) listWorkActivity(c *gin.Context) {
	workID, ok := h.resolveWorkID(c)
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(c.Query("limit"))
	activities, err := h.activities.List(c.Request.Context(), c.Param("type"), workID, limit)
	if err != nil {
		log.Printf("work activity list failed for %s:%d", c.Param("type"), workID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not load work history"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"activities": activities})
}

type workCommentRequest struct {
	Message string `json:"message"`
}

func (h *Handler) createWorkComment(c *gin.Context) {
	workID, ok := h.resolveWorkID(c)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 10<<10)
	var request workCommentRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "comment must be valid JSON and under 10 KB"})
		return
	}
	if _, ok := currentAuthenticatedUser(c); !ok {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	actor, _ := currentAuthenticatedUser(c)
	activity, err := h.activities.Record(c.Request.Context(), actor, c.Param("type"), workID, "comment", request.Message)
	if err != nil {
		if errors.Is(err, usecase.ErrInvalidActivity) || errors.Is(err, usecase.ErrInvalidWorkType) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "comment must contain 1 to 2,000 characters"})
			return
		}
		log.Printf("work comment save failed for %s:%d", c.Param("type"), workID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not save comment"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"activity": activity})
}

func (h *Handler) recordWorkActivity(c *gin.Context, workType string, workID uint, kind, message string) {
	if h.activities == nil || workID == 0 {
		return
	}
	actor, ok := currentAuthenticatedUser(c)
	if !ok {
		return
	}
	if _, err := h.activities.Record(c.Request.Context(), actor, workType, workID, kind, message); err != nil {
		log.Printf("could not record %s activity for %s:%d", kind, workType, workID)
	}
}

func (h *Handler) listWorkAttachments(c *gin.Context) {
	workID, ok := h.resolveWorkID(c)
	if !ok {
		return
	}
	attachments, err := h.attachments.List(c.Request.Context(), strings.ToLower(c.Param("type")), workID)
	if err != nil {
		log.Printf("attachment list failed for %s:%d", c.Param("type"), workID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not load attachments"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"attachments": attachments})
}

func (h *Handler) uploadWorkAttachment(c *gin.Context) {
	workID, ok := h.resolveWorkID(c)
	if !ok {
		return
	}
	const maxRequestBytes = usecase.MaxAttachmentBytes + (1 << 20)
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxRequestBytes)
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		var tooLarge *http.MaxBytesError
		if c.Request.MultipartForm != nil {
			defer c.Request.MultipartForm.RemoveAll()
		}
		if errors.As(err, &tooLarge) || errors.Is(err, multipart.ErrMessageTooLarge) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "file exceeds the 10 MB limit"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "choose a file to upload"})
		return
	}
	defer file.Close()
	if c.Request.MultipartForm != nil {
		defer c.Request.MultipartForm.RemoveAll()
	}
	actor, ok := currentAuthenticatedUser(c)
	if !ok {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	workType := strings.ToLower(c.Param("type"))
	attachment, err := h.attachments.Upload(c.Request.Context(), actor, workType, workID, header.Filename, file)
	if err != nil {
		if errors.Is(err, usecase.ErrAttachmentTooLarge) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "file exceeds the 10 MB limit"})
			return
		}
		if errors.Is(err, usecase.ErrInvalidAttachment) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "use a valid PDF, image, TXT, or CSV file (maximum 10 MB)"})
			return
		}
		log.Printf("attachment upload failed for %s:%d", workType, workID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not save attachment"})
		return
	}
	h.recordWorkActivity(c, workType, workID, "attachment_added", "Attachment uploaded: "+attachment.OriginalName)
	c.JSON(http.StatusCreated, gin.H{"attachment": attachment})
}

func (h *Handler) downloadWorkAttachment(c *gin.Context) {
	workID, ok := h.resolveWorkID(c)
	if !ok {
		return
	}
	parsedID, err := strconv.ParseUint(c.Param("attachmentID"), 10, 64)
	if err != nil || parsedID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid attachment ID"})
		return
	}
	attachment, file, err := h.attachments.Download(c.Request.Context(), strings.ToLower(c.Param("type")), workID, uint(parsedID))
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "attachment not found"})
			return
		}
		log.Printf("attachment download failed for %s:%d attachment:%d", c.Param("type"), workID, parsedID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not download attachment"})
		return
	}
	defer file.Close()
	disposition := mime.FormatMediaType("attachment", map[string]string{"filename": attachment.OriginalName})
	c.Header("Content-Type", attachment.ContentType)
	c.Header("Content-Disposition", disposition)
	c.Header("Content-Length", strconv.FormatInt(attachment.Size, 10))
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Cache-Control", "private, no-store")
	c.Status(http.StatusOK)
	if _, err := io.Copy(c.Writer, file); err != nil {
		log.Printf("attachment response interrupted for %s:%d attachment:%d", c.Param("type"), workID, parsedID)
	}
}

func writeSecurityError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, usecase.ErrInvalidUser), errors.Is(err, usecase.ErrWeakPassword), errors.Is(err, usecase.ErrPasswordTooLong):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, usecase.ErrUsernameTaken):
		c.JSON(http.StatusConflict, gin.H{"error": "username is already in use"})
	case errors.Is(err, usecase.ErrLastAdmin):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	case errors.Is(err, domain.ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
	default:
		log.Printf("user management failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not save user changes"})
	}
}

func currentAuthenticatedUser(c *gin.Context) (domain.User, bool) {
	value, ok := c.Get(authenticatedUserKey)
	user, valid := value.(domain.User)
	return user, ok && valid
}

func currentCreatorName(c *gin.Context) string {
	if user, ok := currentAuthenticatedUser(c); ok {
		if displayName := strings.TrimSpace(user.DisplayName); displayName != "" {
			return displayName
		}
		if username := strings.TrimSpace(user.Username); username != "" {
			return username
		}
	}
	if username, _, ok := c.Request.BasicAuth(); ok && strings.TrimSpace(username) != "" {
		return strings.TrimSpace(username)
	}
	return "Anonymous user"
}

func (h *Handler) auditWrites() gin.HandlerFunc {
	return func(c *gin.Context) {
		method := c.Request.Method
		if method != http.MethodPost && method != http.MethodPut && method != http.MethodPatch && method != http.MethodDelete {
			c.Next()
			return
		}
		c.Next()
		user, ok := currentAuthenticatedUser(c)
		if !ok {
			return
		}
		resource := c.Request.URL.Path
		if len(resource) > 240 {
			resource = resource[:240]
		}
		userAgent := c.Request.UserAgent()
		if len(userAgent) > 256 {
			userAgent = userAgent[:256]
		}
		event := domain.AuditEvent{
			UserID: user.ID, Username: user.Username, Role: user.Role,
			Action: method, Resource: resource, StatusCode: c.Writer.Status(),
			IPAddress: c.ClientIP(), UserAgent: userAgent, CreatedAt: time.Now().UTC(),
		}
		if err := h.security.RecordAudit(c.Request.Context(), event); err != nil {
			log.Printf("could not record audit event for %s %s", method, resource)
		}
	}
}

func (h *Handler) assistantStatus(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"enabled": true, "ai_enabled": h.assistant.Enabled()})
}

func (h *Handler) assistantTranscribe(c *gin.Context) {
	if !h.assistant.Enabled() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Groq API is not configured yet"})
		return
	}
	if !h.allowAssistantRequest(c.ClientIP(), time.Now()) {
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "Too many voice requests. Wait a minute and try again."})
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 12<<20)
	file, header, err := c.Request.FormFile("audio")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "audio recording is missing or too large"})
		return
	}
	defer file.Close()
	if header.Size == 0 || header.Size > 10<<20 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "audio must be between 1 byte and 10 MB"})
		return
	}
	language := strings.ToLower(strings.TrimSpace(c.PostForm("language")))
	if language == "" {
		language = "auto"
	}
	if language != "auto" && language != "bn" && language != "en" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "language must be auto, bn, or en"})
		return
	}
	transcript, err := h.assistant.Transcribe(c.Request.Context(), header.Filename, file, language)
	if err != nil {
		log.Printf("assistant transcription failed: %v", err)
		if isAssistantRateLimited(err) {
			c.Header("Retry-After", "60")
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "Voice assistant is busy right now. Please wait a minute and try again."})
			return
		}
		c.JSON(http.StatusBadGateway, gin.H{"error": "speech transcription failed; check Groq limits and try again"})
		return
	}
	if !containsSpeechWords(transcript) {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "no speech was detected; please speak louder and try again"})
		return
	}
	if len([]rune(transcript)) > 1200 {
		transcript = string([]rune(transcript)[:1200])
	}
	c.JSON(http.StatusOK, gin.H{"text": transcript})
}

func containsSpeechWords(text string) bool {
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			return true
		}
	}
	return false
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

var assistantAssigneeAliases = map[string]string{
	"jaber":         "Mabubul Alam Patwary",
	"jaber patwary": "Mabubul Alam Patwary",
}

func resolveAssistantAssignee(value string) string {
	key := normalizeAssistantPersonName(value)
	for _, name := range assistantAssignees {
		if normalizeAssistantPersonName(name) == key {
			return name
		}
	}
	return assistantAssigneeAliases[key]
}

// resolveAssistantAssigneeMention accepts a unique first-name mention while
// still resolving it against the configured IT team only.
func resolveAssistantAssigneeMention(value string) string {
	value = strings.TrimSpace(strings.Trim(value, " \t\r\n.,:;()\"'"))
	if name := resolveAssistantAssignee(value); name != "" {
		return name
	}
	key := normalizeAssistantPersonName(value)
	if key == "" {
		return ""
	}
	var match string
	for _, name := range assistantAssignees {
		canonical := normalizeAssistantPersonName(name)
		if strings.HasPrefix(canonical, key+" ") {
			if match != "" {
				return ""
			}
			match = name
		}
	}
	return match
}

func resolveTaskAssigneeMention(value string) string {
	value = strings.TrimSpace(strings.Trim(value, " \t\r\n.,:;()\"'"))
	if strings.EqualFold(value, usecase.DefaultTaskAssignee) {
		return usecase.DefaultTaskAssignee
	}
	return resolveAssistantAssigneeMention(value)
}

func normalizeAssistantValue(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(value), " "))
}

func normalizeAssistantPersonName(value string) string {
	var tokens []string
	for _, token := range assistantNameTokens(value) {
		switch token {
		case "bhai", "vai", "bhaiya", "bhaiyer", "vaiyer", "apu", "apa", "sir", "madam", "mr", "mrs", "miss", "md", "er", "ar", "r", "জনাব", "ভাই", "ভাইয়ের", "ভাইয়ের", "এর", "র", "আপু", "আপা", "স্যার", "ম্যাডাম":
			continue
		}
		tokens = append(tokens, token)
	}
	return strings.Join(tokens, " ")
}

func assistantEmployeeNameMatches(candidate, requested string) bool {
	wanted := strings.Fields(normalizeAssistantPersonName(requested))
	if len(wanted) == 0 {
		return false
	}
	canonical := strings.Fields(normalizeAssistantPersonName(candidate))
	if strings.Join(canonical, " ") == strings.Join(wanted, " ") {
		return true
	}
	canonicalSet := make(map[string]struct{}, len(canonical))
	for _, token := range canonical {
		canonicalSet[token] = struct{}{}
	}
	for _, token := range wanted {
		if _, ok := canonicalSet[token]; !ok {
			return false
		}
	}
	return true
}

func findAssistantEmployeeMatches(requested string, employees []domain.Employee, limit int) []domain.Employee {
	matches := make([]domain.Employee, 0)
	for _, employee := range employees {
		if assistantEmployeeNameMatches(employee.Name, requested) {
			matches = append(matches, employee)
		}
	}
	if limit > 0 && len(matches) > limit {
		matches = matches[:limit]
	}
	return matches
}

func isAssistantRateLimited(err error) bool {
	return err != nil && strings.Contains(err.Error(), "HTTP 429")
}

// fallbackTicketDetails keeps chat usable when Groq is unavailable. It uses the
// employee directory and the IT team list, while preserving the user's issue text.
func fallbackTicketDetails(messages []groqapi.Message, employees []domain.Employee, assignees []string) groqapi.TicketDetails {
	var text strings.Builder
	for _, message := range messages {
		if message.Role == "user" && !strings.HasPrefix(strings.ToLower(message.Content), "employee selected by user:") {
			text.WriteByte(' ')
			text.WriteString(message.Content)
		}
	}
	all := strings.TrimSpace(text.String())
	result := groqapi.TicketDetails{}
	for _, employee := range employees {
		name := strings.TrimSpace(employee.Name)
		if name != "" && strings.Contains(strings.ToLower(all), strings.ToLower(name)) {
			result.EmployeeName = name
			break
		}
	}
	if result.EmployeeName == "" {
		matches := relevantAssistantEmployees(messages, employees, 2)
		if len(matches) == 1 {
			result.EmployeeName = matches[0].Name
		}
	}
	result.Assignee = fallbackAssigneeFromText(all)
	problem := all
	if result.EmployeeName != "" {
		problem = strings.ReplaceAll(problem, result.EmployeeName, "")
	}
	problem = regexp.MustCompile(`(?i)\bassign(?:ed)?\s*(?:to|by)?\s*[\pL.]+(?:\s+[\pL.]+){0,5}\s*$`).ReplaceAllString(problem, "")
	problem = strings.TrimSpace(strings.Trim(problem, " ,:;.-"))
	result.Problem = problem
	return result
}

func fallbackAssigneeFromText(text string) string {
	match := regexp.MustCompile(`(?i)\b(?:assign|assing)(?:ed|ee|ing)?\s*(?:to\s*)?(.+)$`).FindStringSubmatch(text)
	if len(match) > 1 {
		clause := regexp.MustCompile(`(?i)\b(?:for|because|priority|due|task details|description)\b.*$`).ReplaceAllString(match[1], "")
		clause = strings.TrimSpace(strings.Trim(clause, " \t\r\n.,:;()\"'"))
		if name := resolveAssistantAssigneeMention(clause); name != "" {
			return name
		}
		words := strings.Fields(clause)
		for size := 1; size <= len(words) && size <= 5; size++ {
			if name := resolveAssistantAssigneeMention(strings.Join(words[:size], " ")); name != "" {
				return name
			}
		}
	}
	if m := regexp.MustCompile(`(?i)([\pL.]+(?:\s+[\pL.]+){0,4})\s+ke\s+dao\b`).FindStringSubmatch(text); len(m) > 1 {
		return resolveAssistantAssigneeMention(m[1])
	}
	return ""
}

func stripTaskAssignmentClause(text string) string {
	if fallbackTaskAssigneeFromText(text) == "" {
		return text
	}
	if index := regexp.MustCompile(`(?i)\b(?:assign|assing)(?:ed|ee|ing)?\s*(?:to\s*)?`).FindStringIndex(text); index != nil {
		return text[:index[0]]
	}
	if index := regexp.MustCompile(`(?i)[\pL.]+(?:\s+[\pL.]+){0,4}\s+ke\s+dao\b`).FindStringIndex(text); index != nil {
		return text[:index[0]]
	}
	return text
}

func fallbackTaskAssigneeFromText(text string) string {
	if assignee := fallbackAssigneeFromText(text); assignee != "" {
		return assignee
	}
	match := regexp.MustCompile(`(?i)\b(?:assign|assing)(?:ed|ee|ing)?\s*(?:to\s*)?(it\s*team)\b`).FindStringSubmatch(text)
	if len(match) > 1 {
		return usecase.DefaultTaskAssignee
	}
	return ""
}

func isTaskGreeting(text string) bool {
	value := strings.ToLower(strings.TrimSpace(text))
	value = strings.Trim(value, " .,!?:;\t\r\n")
	switch value {
	case "hi", "hello", "hey", "hii", "helo", "good morning", "good afternoon", "good evening", "salam", "assalamu alaikum", "assalamualaikum", "আসসালামু আলাইকুম", "হ্যালো", "হাই":
		return true
	default:
		return false
	}
}

func taskAssistantPromptSlot(text string) string {
	value := strings.ToLower(text)
	if strings.Contains(value, "who gave") || strings.Contains(value, "given by") || strings.Contains(value, "who requested") || strings.Contains(value, "task creator") {
		return "creator"
	}
	if strings.Contains(value, "what needs to be done") || strings.Contains(value, "task details") || strings.Contains(value, "describe the work") || strings.Contains(value, "what task") {
		return "description"
	}
	if strings.Contains(value, "assign to") || strings.Contains(value, "who should i assign") || strings.Contains(value, "assignee") || strings.Contains(value, "who to assign") {
		return "assignee"
	}
	if strings.Contains(value, "what needs to be done") || strings.Contains(value, "task details") || strings.Contains(value, "describe the work") || strings.Contains(value, "what task") || strings.Contains(value, "কী কাজ") || strings.Contains(value, "কি কাজ") {
		return "description"
	}
	return ""
}

func extractTaskCreator(text string) string {
	match := regexp.MustCompile(`(?i)(?:given\s*by|requested\s*by|task\s*creator)\s*[:=]?\s*(.+)`).FindStringSubmatch(text)
	if len(match) < 2 {
		return ""
	}
	name := regexp.MustCompile(`(?i)\b(?:task\s*details|description|assign(?:ed|ee|ing)?\s*(?:to)?|priority|due\s*date)\b.*$`).ReplaceAllString(match[1], "")
	if index := strings.IndexAny(name, ",;\n\""); index >= 0 {
		name = name[:index]
	}
	for index := 0; index+1 < len(name); index++ {
		if name[index] != '.' || name[index+1] != ' ' {
			continue
		}
		words := strings.Fields(name[:index])
		if len(words) == 0 {
			continue
		}
		abbreviation := strings.ToLower(strings.Trim(words[len(words)-1], "."))
		if abbreviation == "md" || abbreviation == "mr" || abbreviation == "mrs" || abbreviation == "ms" || abbreviation == "dr" {
			continue
		}
		name = name[:index]
		break
	}
	name = strings.TrimSpace(strings.Trim(name, " .,:;\t\r\n\"'"))
	if isTaskGreeting(name) || len([]rune(name)) > 100 {
		return ""
	}
	return name
}

func extractInlineTaskCreator(text string) (string, string) {
	// Accept a known team member as the leading creator without requiring a
	// "Given by:" label. Unknown leading words stay part of the free-form task.
	text = strings.TrimSpace(text)
	creatorNames := append([]string(nil), assistantAssignees...)
	sort.Slice(creatorNames, func(i, j int) bool { return len(creatorNames[i]) > len(creatorNames[j]) })
	for _, name := range creatorNames {
		if len(text) < len(name) || !strings.EqualFold(text[:len(name)], name) {
			continue
		}
		remainder := text[len(name):]
		if remainder != "" {
			first, _ := utf8.DecodeRuneInString(remainder)
			if unicode.IsLetter(first) || unicode.IsNumber(first) {
				continue
			}
			remainder = regexp.MustCompile(`(?i)^(?:-er|er|r|'s|’s)\s*`).ReplaceAllString(remainder, "")
		}
		remainder = strings.TrimSpace(remainder)
		remainder = regexp.MustCompile(`(?i)^(?:-er|er|r|'s|’s)\s+`).ReplaceAllString(remainder, "")
		return name, cleanTaskDescription(remainder, "")
	}
	return "", ""
}

func removeTaskCreatorMarker(text, creator string) string {
	if creator == "" {
		return text
	}
	pattern := `(?i)(?:given\s*by|requested\s*by|task\s*creator)\s*[:=]?\s*` + regexp.QuoteMeta(creator)
	return regexp.MustCompile(pattern).ReplaceAllString(text, " ")
}

func cleanTaskDescription(text, creator string) string {
	text = removeTaskCreatorMarker(text, creator)
	text = strings.TrimLeft(text, " \t\r\n.,:;")
	text = regexp.MustCompile(`(?i)^\s*(?:task\s*details|description|work)\s*[:=]\s*`).ReplaceAllString(text, "")
	text = stripTaskAssignmentClause(text)
	text = regexp.MustCompile(`(?i)\b(?:priority|due\s*date)\s*[:=]?.*$`).ReplaceAllString(text, " ")
	return strings.TrimSpace(strings.Trim(text, " \t\r\n.,:;\"'"))
}

// fallbackTaskDetails only fills task fields from explicit user input or a
// direct answer to the assistant's current question. AI output is not evidence
// that the user supplied a required field.
func fallbackTaskDetails(messages []groqapi.Message, selectedAssignee string) groqapi.TaskDetails {
	var userText strings.Builder
	createdBy, description := "", ""
	askedSlot := ""
	promptAssignee := ""
	for _, message := range messages {
		content := strings.TrimSpace(message.Content)
		if message.Role == "assistant" {
			if slot := taskAssistantPromptSlot(content); slot != "" {
				askedSlot = slot
			}
			continue
		}
		if message.Role != "user" {
			continue
		}
		slot := askedSlot
		askedSlot = ""
		userText.WriteByte(' ')
		userText.WriteString(content)
		if isTaskGreeting(content) {
			continue
		}

		creator := extractTaskCreator(content)
		inlineDescription := ""
		creatorWasBareAnswer := false
		if creator == "" && slot == "creator" {
			creator, inlineDescription = extractInlineTaskCreator(content)
			creatorWasBareAnswer = creator != "" && inlineDescription == ""
			if creator == "" && inlineDescription == "" {
				creator = resolveAssistantAssigneeMention(content)
				creatorWasBareAnswer = creator != ""
			}
		}
		if slot == "assignee" {
			promptAssignee = resolveTaskAssigneeMention(content)
		}
		if creator != "" {
			createdBy = creator
		}

		if inlineDescription != "" {
			description = inlineDescription
		} else if marker := regexp.MustCompile(`(?i)(?:task\s*details|description|work)\s*[:=]\s*(.*)`).FindStringSubmatch(content); len(marker) > 1 {
			description = cleanTaskDescription(marker[1], "")
		} else if slot == "description" {
			description = cleanTaskDescription(content, creator)
		} else if slot == "assignee" {
			// Assignee-only answers must not replace the task description.
		} else if creator != "" && !creatorWasBareAnswer {
			// Free-form task descriptions remain unrestricted. Remove only the
			// creator and explicit assignment metadata around the user's text.
			desc := cleanTaskDescription(content, creator)
			if desc != "" {
				description = desc
			}
		} else if creator == "" {
			// Given by is optional. Preserve free-form task details and let the
			// task service supply its configured default creator.
			desc := cleanTaskDescription(content, "")
			if desc != "" {
				description = desc
			}
		}
	}

	assignee := resolveTaskAssigneeMention(selectedAssignee)
	if assignee == "" {
		assignee = fallbackTaskAssigneeFromText(userText.String())
	}
	if assignee == "" {
		assignee = promptAssignee
	}
	description = strings.TrimSpace(description)
	title := description
	if len([]rune(title)) > 160 {
		title = string([]rune(title)[:160])
	}
	return groqapi.TaskDetails{Title: title, Description: description, CreatedBy: createdBy, Assignee: assignee, Priority: "medium"}
}

func relevantAssistantEmployees(messages []groqapi.Message, employees []domain.Employee, limit int) []domain.Employee {
	if limit <= 0 || len(employees) == 0 {
		return nil
	}
	var queryBuilder strings.Builder
	for _, message := range messages {
		if strings.EqualFold(message.Role, "user") {
			queryBuilder.WriteByte(' ')
			queryBuilder.WriteString(strings.ToLower(message.Content))
		}
	}
	query := queryBuilder.String()
	queryTokens := assistantNameTokens(query)
	if len(queryTokens) == 0 {
		return nil
	}
	type rankedEmployee struct {
		employee domain.Employee
		score    float64
	}
	ranked := make([]rankedEmployee, 0)
	for _, employee := range employees {
		name := strings.TrimSpace(employee.Name)
		if name == "" {
			continue
		}
		if strings.Contains(query, normalizeAssistantValue(name)) {
			ranked = append(ranked, rankedEmployee{employee: employee, score: 2})
			continue
		}
		var tokenScores []float64
		for _, nameToken := range assistantNameTokens(name) {
			if len([]rune(nameToken)) < 3 || nameToken == "mohd" || nameToken == "md" {
				continue
			}
			best := 0.0
			for _, queryToken := range queryTokens {
				if len([]rune(queryToken)) < 3 {
					continue
				}
				if score := assistantTokenSimilarity(nameToken, queryToken); score > best {
					best = score
				}
			}
			if best >= 0.72 {
				tokenScores = append(tokenScores, best)
			}
		}
		if len(tokenScores) == 0 {
			continue
		}
		sort.Sort(sort.Reverse(sort.Float64Slice(tokenScores)))
		score := tokenScores[0]
		if len(tokenScores) > 1 {
			score = (tokenScores[0] + tokenScores[1]) / 2
		}
		ranked = append(ranked, rankedEmployee{employee: employee, score: score})
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].score == ranked[j].score {
			return ranked[i].employee.Name < ranked[j].employee.Name
		}
		return ranked[i].score > ranked[j].score
	})
	if len(ranked) > limit {
		ranked = ranked[:limit]
	}
	result := make([]domain.Employee, 0, len(ranked))
	for _, candidate := range ranked {
		result = append(result, candidate.employee)
	}
	return result
}

func assistantNameTokens(value string) []string {
	return strings.FieldsFunc(strings.ToLower(value), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})
}

func assistantTokenSimilarity(left, right string) float64 {
	leftRunes, rightRunes := []rune(left), []rune(right)
	maxLen := len(leftRunes)
	if len(rightRunes) > maxLen {
		maxLen = len(rightRunes)
	}
	if maxLen == 0 {
		return 0
	}
	previous := make([]int, len(rightRunes)+1)
	for j := range previous {
		previous[j] = j
	}
	for i, leftRune := range leftRunes {
		current := make([]int, len(rightRunes)+1)
		current[0] = i + 1
		for j, rightRune := range rightRunes {
			cost := 0
			if leftRune != rightRune {
				cost = 1
			}
			current[j+1] = min(current[j]+1, previous[j+1]+1, previous[j]+cost)
		}
		previous = current
	}
	return 1 - float64(previous[len(rightRunes)])/float64(maxLen)
}

func (h *Handler) assistantChat(c *gin.Context) {
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
		message.Content = normalizeAssistantBanglish(message.Content)
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
	employeeCandidates := relevantAssistantEmployees(request.Messages, h.tickets.ListEmployees(), 8)
	var details groqapi.TicketDetails
	if h.assistant.Enabled() {
		var err error
		details, err = h.assistant.ExtractTicket(c.Request.Context(), request.Messages, employeeCandidates, assistantAssignees)
		if err != nil {
			log.Printf("assistant extraction failed; using programming fallback: %v", err)
		}
	}
	fallback := fallbackTicketDetails(request.Messages, h.tickets.ListEmployees(), assistantAssignees)
	if details.EmployeeName == "" {
		details.EmployeeName = fallback.EmployeeName
	}
	if details.Problem == "" {
		details.Problem = fallback.Problem
	}
	if details.Assignee == "" {
		details.Assignee = fallback.Assignee
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
		exact := findAssistantEmployeeMatches(details.EmployeeName, matches, usecase.MaxSearchResults)
		if len(exact) == 0 {
			exact = findAssistantEmployeeMatches(details.EmployeeName, h.tickets.ListEmployees(), usecase.MaxSearchResults)
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
		Problem: details.Problem, CreatedBy: currentCreatorName(c), AssignedTo: details.Assignee,
	})
	if err != nil {
		h.writeCreateError(c, err)
		return
	}
	h.recordWorkActivity(c, "ticket", ticket.ID, "created", "Ticket created for "+ticket.EmployeeName)
	reply := fmt.Sprintf("Pending ticket %s তৈরি হয়েছে।", ticket.TicketNumber)
	if ticket.AssignedTo != "" {
		reply += " Assignee: " + ticket.AssignedTo + "."
	}
	c.JSON(http.StatusCreated, gin.H{"reply": reply, "ticket": toView(ticket, emailQueued), "created": true})
}

func (h *Handler) assistantTaskChat(c *gin.Context) {
	if h.tasks == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Task service is unavailable"})
		return
	}
	if !h.allowAssistantRequest(c.ClientIP(), time.Now()) {
		c.Header("Retry-After", "60")
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "Task assistant is busy. Please wait a minute and try again."})
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 20<<10)
	var request assistantChatRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid task assistant request"})
		return
	}
	if len(request.Messages) == 0 || len(request.Messages) > 12 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "send between 1 and 12 messages"})
		return
	}
	contentLength := 0
	for i, message := range request.Messages {
		message.Role = strings.ToLower(strings.TrimSpace(message.Role))
		message.Content = strings.TrimSpace(message.Content)
		message.Content = normalizeAssistantBanglish(message.Content)
		if (message.Role != "user" && message.Role != "assistant") || message.Content == "" || len([]rune(message.Content)) > 1200 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "each message must have a valid role and 1 to 1200 characters"})
			return
		}
		contentLength += len(message.Content)
		request.Messages[i] = message
	}
	if contentLength > 6000 || request.Messages[len(request.Messages)-1].Role != "user" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "conversation is too long or does not end with a user message"})
		return
	}
	fallback := fallbackTaskDetails(request.Messages, request.SelectedAssignee)
	if fallback.CreatedBy == "" {
		fallback.CreatedBy = usecase.DefaultTaskCreator
	}
	if fallback.Description == "" {
		c.JSON(http.StatusOK, gin.H{"reply": "What needs to be done? Please describe the task details."})
		return
	}
	if len([]rune(fallback.Description)) > 2000 {
		c.JSON(http.StatusOK, gin.H{"reply": "Please keep the task description under 2,000 characters."})
		return
	}
	assignee := fallback.Assignee
	if selected := strings.TrimSpace(request.SelectedAssignee); selected != "" {
		assignee = resolveTaskAssigneeMention(selected)
		if assignee == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "selected assignee is invalid; choose an IT Team member or IT Team"})
			return
		}
	}
	if assignee == "" {
		assignee = usecase.DefaultTaskAssignee
	}
	var details groqapi.TaskDetails
	if h.assistant.Enabled() {
		var err error
		details, err = h.assistant.ExtractTask(c.Request.Context(), request.Messages, assistantAssignees)
		if err != nil {
			log.Printf("task assistant extraction failed; using programming fallback: %v", err)
		}
	}
	if details.Title == "" {
		details.Title = fallback.Title
	}
	if details.Description == "" {
		details.Description = fallback.Description
	}
	if details.CreatedBy == "" {
		details.CreatedBy = fallback.CreatedBy
	}
	if details.Assignee == "" {
		details.Assignee = fallback.Assignee
	}
	details.Title = strings.TrimSpace(details.Title)
	details.Description = strings.TrimSpace(details.Description)
	details.CreatedBy = strings.TrimSpace(details.CreatedBy)
	details.Assignee = strings.TrimSpace(details.Assignee)
	details.Priority = "medium"
	details.DueDate = ""
	// Task creation attribution always comes from the authenticated account.
	details.Title = fallback.Title
	details.Description = fallback.Description
	details.CreatedBy = currentCreatorName(c)
	details.Assignee = assignee
	if selected := strings.TrimSpace(request.SelectedAssignee); selected != "" {
		details.Assignee = resolveTaskAssigneeMention(selected)
		if details.Assignee == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "selected assignee is invalid; choose an IT Team member or IT Team"})
			return
		}
	} else if details.Assignee != "" {
		details.Assignee = resolveTaskAssigneeMention(details.Assignee)
	}
	if len([]rune(details.CreatedBy)) > 256 {
		c.JSON(http.StatusOK, gin.H{"reply": "আপনার account display name ২৫৬ অক্ষরের বেশি; admin panel থেকে এটি ছোট করুন।"})
		return
	}
	if details.Description == "" {
		details.Description = details.Title
	}
	if details.Description == "" {
		c.JSON(http.StatusOK, gin.H{"reply": "এবার task-এ কী কাজ করতে হবে, সেটি বলুন।"})
		return
	}
	if len([]rune(details.Description)) > 2000 {
		c.JSON(http.StatusOK, gin.H{"reply": "Task-এর কাজের বিবরণ ২০০০ অক্ষরের মধ্যে ছোট করে বলুন।"})
		return
	}
	if details.Title == "" {
		details.Title = strings.TrimSpace(strings.SplitN(details.Description, ".", 2)[0])
		if len([]rune(details.Title)) > 160 {
			details.Title = string([]rune(details.Title)[:160])
		}
	}
	if details.Assignee == "" {
		details.Assignee = usecase.DefaultTaskAssignee
	}
	if len([]rune(details.Title)) > 160 {
		c.JSON(http.StatusOK, gin.H{"reply": "Task-এর শিরোনামটি ১৬০ অক্ষরের মধ্যে ছোট করে বলুন।"})
		return
	}
	task, err := h.tasks.Create(c.Request.Context(), usecase.CreateTaskInput{
		Title: details.Title, Description: details.Description, CreatedBy: currentCreatorName(c),
		AssignedTo: details.Assignee, Priority: details.Priority, DueDate: details.DueDate,
	})
	if err != nil {
		log.Printf("task assistant save failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not create task"})
		return
	}
	h.recordWorkActivity(c, "task", task.ID, "created", "Task created: "+task.Title)
	reply := fmt.Sprintf("Task TSK-%06d created.", task.ID)
	if task.AssignedTo != "" {
		reply += " Assigned to " + task.AssignedTo + "."
	}
	c.JSON(http.StatusCreated, gin.H{"reply": reply, "task": toTaskView(task), "created": true})
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
	AssetTag   string `json:"asset_tag"`
	AssignedTo string `json:"assigned_to"`
	Priority   string `json:"priority"`
}

type assignTicketRequest struct {
	Assignee string `json:"assignee"`
}

type setTicketStatusRequest struct {
	Status string `json:"status"`
}

func (h *Handler) setTicketStatus(c *gin.Context) {
	var request setTicketStatusRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON request"})
		return
	}
	previous, previousErr := h.tickets.Get(c.Request.Context(), c.Param("id"))
	ticket, emailQueued, err := h.tickets.SetStatus(c.Request.Context(), c.Param("id"), request.Status)
	switch err {
	case nil:
		if previousErr == nil && previous.Status != ticket.Status {
			h.recordWorkActivity(c, "ticket", ticket.ID, "status_changed", fmt.Sprintf("Status changed: %s → %s", previous.Status, ticket.Status))
		}
		c.JSON(http.StatusOK, toView(ticket, emailQueued))
	case usecase.ErrInvalidStatus:
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case domain.ErrNotFound:
		c.JSON(http.StatusNotFound, gin.H{"error": "ticket not found"})
	default:
		log.Printf("ticket status update failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not update ticket status"})
	}
}

func (h *Handler) assignTicket(c *gin.Context) {
	var request assignTicketRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON request"})
		return
	}
	previous, previousErr := h.tickets.Get(c.Request.Context(), c.Param("id"))
	ticket, err := h.tickets.Assign(c.Request.Context(), c.Param("id"), request.Assignee)
	switch err {
	case nil:
		if previousErr == nil && previous.AssignedTo != ticket.AssignedTo {
			oldAssignee := previous.AssignedTo
			if strings.TrimSpace(oldAssignee) == "" {
				oldAssignee = "Unassigned"
			}
			newAssignee := ticket.AssignedTo
			if strings.TrimSpace(newAssignee) == "" {
				newAssignee = "Unassigned"
			}
			h.recordWorkActivity(c, "ticket", ticket.ID, "assignee_changed", fmt.Sprintf("Assignee changed: %s → %s", oldAssignee, newAssignee))
		}
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

type setTicketPriorityRequest struct {
	Priority string `json:"priority"`
}

func (h *Handler) setTicketPriority(c *gin.Context) {
	var request setTicketPriorityRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON request"})
		return
	}
	previous, previousErr := h.tickets.Get(c.Request.Context(), c.Param("id"))
	ticket, err := h.tickets.SetPriority(c.Request.Context(), c.Param("id"), request.Priority)
	switch err {
	case nil:
		if previousErr == nil && previous.Priority != ticket.Priority {
			h.recordWorkActivity(c, "ticket", ticket.ID, "priority_changed", fmt.Sprintf("Priority changed: %s → %s", previous.Priority, ticket.Priority))
		}
		c.JSON(http.StatusOK, toView(ticket, false))
	case usecase.ErrInvalidPriority:
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case usecase.ErrSLAPaused:
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	case domain.ErrNotFound:
		c.JSON(http.StatusNotFound, gin.H{"error": "ticket not found"})
	default:
		log.Printf("ticket priority update failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not update ticket priority"})
	}
}

func (h *Handler) setTicketSLAPause(c *gin.Context) {
	var request struct {
		Paused bool `json:"paused"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON request"})
		return
	}
	previous, _ := h.tickets.Get(c.Request.Context(), c.Param("id"))
	ticket, err := h.tickets.SetSLAPaused(c.Request.Context(), c.Param("id"), request.Paused)
	switch err {
	case nil:
		if (previous.SLAPausedAt == nil) != (ticket.SLAPausedAt == nil) {
			message := "SLA timer resumed"
			if ticket.SLAPausedAt != nil {
				message = "SLA timer paused while waiting"
			}
			h.recordWorkActivity(c, "ticket", ticket.ID, "sla_changed", message)
		}
		c.JSON(http.StatusOK, toView(ticket, false))
	case usecase.ErrInvalidStatus:
		c.JSON(http.StatusConflict, gin.H{"error": "completed tickets cannot pause or resume an SLA"})
	case domain.ErrNotFound:
		c.JSON(http.StatusNotFound, gin.H{"error": "ticket not found"})
	default:
		log.Printf("ticket SLA update failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not update ticket SLA"})
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
		Problem: request.Problem, AssetTag: request.AssetTag, CreatedBy: currentCreatorName(c), AssignedTo: request.AssignedTo, Priority: request.Priority,
	})
	if err != nil {
		h.writeCreateError(c, err)
		return
	}
	h.recordWorkActivity(c, "ticket", ticket.ID, "created", "Ticket created for "+ticket.EmployeeName)
	c.JSON(http.StatusCreated, toView(ticket, emailQueued))
}

func (h *Handler) writeCreateError(c *gin.Context, err error) {
	switch {
	case err == usecase.ErrInvalidStatus:
		c.JSON(http.StatusBadRequest, gin.H{"error": "status must be 'pending', 'in_progress', or 'done'"})
	case err == usecase.ErrEmployeeMissing:
		c.JSON(http.StatusBadRequest, gin.H{"error": "employee name is required"})
	case err == usecase.ErrEmployeeUnknown:
		c.JSON(http.StatusNotFound, gin.H{"error": "employee not found"})
	case err == usecase.ErrProblemRequired, err == usecase.ErrProblemTooLong, err == usecase.ErrAssigneeTooLong, err == usecase.ErrAssetTagTooLong, err == usecase.ErrInvalidPriority:
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
	h.recordWorkActivity(c, "ticket", result.Ticket.ID, "status_changed", "Status changed to done")
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
	offset, err := strconv.Atoi(c.DefaultQuery("offset", "0"))
	if err != nil || offset < 0 || offset > 1000000 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "offset must be between 0 and 1,000,000"})
		return
	}
	status := strings.ToLower(strings.TrimSpace(c.Query("status")))
	var from, until time.Time
	if c.Query("from") != "" || c.Query("until") != "" {
		var parseErr error
		from, parseErr = time.Parse(time.RFC3339, c.Query("from"))
		if parseErr != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "from must be an RFC3339 timestamp"})
			return
		}
		until, parseErr = time.Parse(time.RFC3339, c.Query("until"))
		if parseErr != nil || !until.After(from) || until.Sub(from) > 24*time.Hour+time.Minute {
			c.JSON(http.StatusBadRequest, gin.H{"error": "until must be after from and within one day"})
			return
		}
	}
	tickets, total, err := h.tickets.Page(c.Request.Context(), status, from, until, c.Query("q"), limit, offset)
	if err != nil {
		if err == usecase.ErrInvalidFilter {
			c.JSON(http.StatusBadRequest, gin.H{"error": "status must be 'pending', 'in_progress', or 'done'"})
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
	c.JSON(http.StatusOK, gin.H{"tickets": views, "count": len(views), "total": total, "limit": limit, "offset": offset})
}

const MaxCSVExportRows = 10000

func (h *Handler) exportTicketsCSV(c *gin.Context) {
	status := strings.ToLower(strings.TrimSpace(c.Query("status")))
	var from, until time.Time
	if c.Query("from") != "" || c.Query("until") != "" {
		var err error
		from, err = time.Parse(time.RFC3339, c.Query("from"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "from must be an RFC3339 timestamp"})
			return
		}
		until, err = time.Parse(time.RFC3339, c.Query("until"))
		if err != nil || !until.After(from) || until.Sub(from) > 24*time.Hour+time.Minute {
			c.JSON(http.StatusBadRequest, gin.H{"error": "until must be after from and within one day"})
			return
		}
	}
	tickets, total, err := h.tickets.Page(c.Request.Context(), status, from, until, c.Query("q"), MaxCSVExportRows, 0)
	if errors.Is(err, usecase.ErrInvalidFilter) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "status must be pending, in_progress, or done"})
		return
	}
	if err != nil {
		log.Printf("ticket CSV export failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not export tickets"})
		return
	}
	if total > MaxCSVExportRows {
		c.JSON(http.StatusConflict, gin.H{"error": "More than 10,000 tickets match. Narrow the date or status filter and export again."})
		return
	}
	if len(tickets) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "No tickets match the current filters."})
		return
	}
	rows := make([][]string, 0, len(tickets))
	for _, ticket := range tickets {
		rows = append(rows, []string{
			strconv.FormatUint(uint64(ticket.ID), 10), ticket.TicketNumber, ticket.EmployeeName,
			ticket.EmployeeID, ticket.Department, ticket.Problem, ticket.AssetTag, ticket.CreatedBy, ticket.AssignedTo,
			ticket.Priority, ticket.SLADueAt.UTC().Format(time.RFC3339), ticket.Status,
			ticket.CreatedAt.UTC().Format(time.RFC3339),
		})
	}
	filename := "tickets-" + time.Now().UTC().Format("20060102-150405") + ".csv"
	writeCSVResponse(c, filename, []string{"ID", "Ticket number", "Employee", "Employee ID", "Department", "Problem", "Asset tag", "Created by", "Assigned to", "Priority", "SLA due (UTC)", "Status", "Created at (UTC)"}, rows)
}

type createTaskRequest struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	CreatedBy   string `json:"created_by"`
	AssignedTo  string `json:"assigned_to"`
	Priority    string `json:"priority"`
	DueDate     string `json:"due_date"`
}

type updateTaskRequest struct {
	Title       *string `json:"title"`
	Description *string `json:"description"`
	CreatedBy   *string `json:"created_by"`
	AssignedTo  *string `json:"assigned_to"`
	Priority    *string `json:"priority"`
	DueDate     *string `json:"due_date"`
	Status      *string `json:"status"`
}

func (h *Handler) listTasks(c *gin.Context) {
	if h.tasks == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "task service is unavailable"})
		return
	}
	createdFrom, createdUntil, ok := parseTaskDateRange(c)
	if !ok {
		return
	}
	limit, err := strconv.Atoi(c.DefaultQuery("limit", "100"))
	if err != nil || limit <= 0 || limit > usecase.MaxTaskLimit {
		limit = 100
	}
	offset, err := strconv.Atoi(c.DefaultQuery("offset", "0"))
	if err != nil || offset < 0 || offset > 1000000 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "offset must be between 0 and 1,000,000"})
		return
	}
	tasks, total, err := h.tasks.List(c.Request.Context(), domain.TaskFilter{
		Status: c.Query("status"), Assignee: c.Query("assignee"), Query: c.Query("q"),
		CreatedFrom: createdFrom, CreatedUntil: createdUntil, Limit: limit, Offset: offset,
	})
	if err != nil {
		if err == usecase.ErrTaskInvalidStatus {
			c.JSON(http.StatusBadRequest, gin.H{"error": "status must be pending, in_progress, or done"})
			return
		}
		log.Printf("task list failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not load tasks"})
		return
	}
	views := make([]taskView, 0, len(tasks))
	for _, task := range tasks {
		views = append(views, toTaskView(task))
	}
	c.JSON(http.StatusOK, gin.H{"tasks": views, "count": len(views), "total": total, "limit": limit, "offset": offset})
}

func parseTaskDateRange(c *gin.Context) (time.Time, time.Time, bool) {
	fromValue, untilValue := strings.TrimSpace(c.Query("from")), strings.TrimSpace(c.Query("until"))
	if fromValue == "" && untilValue == "" {
		return time.Time{}, time.Time{}, true
	}
	from, err := time.Parse(time.RFC3339, fromValue)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "from must be an RFC3339 timestamp"})
		return time.Time{}, time.Time{}, false
	}
	until, err := time.Parse(time.RFC3339, untilValue)
	if err != nil || !until.After(from) || until.Sub(from) > 24*time.Hour+time.Minute {
		c.JSON(http.StatusBadRequest, gin.H{"error": "until must be after from and within one day"})
		return time.Time{}, time.Time{}, false
	}
	return from, until, true
}

func taskCreatedAtInRange(createdAt, from, until time.Time) bool {
	return (from.IsZero() || !createdAt.Before(from)) && (until.IsZero() || createdAt.Before(until))
}

func (h *Handler) exportTasksCSV(c *gin.Context) {
	if h.tasks == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "task service is unavailable"})
		return
	}
	createdFrom, createdUntil, ok := parseTaskDateRange(c)
	if !ok {
		return
	}
	query := strings.TrimSpace(c.Query("q"))
	status := strings.ToLower(strings.TrimSpace(c.Query("status")))
	var tasks []domain.Task
	var total int64
	var err error
	if strings.HasPrefix(strings.ToLower(query), "tsk-") {
		idValue, parseErr := strconv.ParseUint(strings.TrimPrefix(strings.ToLower(query), "tsk-"), 10, 64)
		getter, ok := h.tasks.(taskGetter)
		if parseErr == nil && idValue > 0 && ok {
			if status != "" && status != domain.StatusPending && status != domain.StatusInProgress && status != domain.StatusDone {
				c.JSON(http.StatusBadRequest, gin.H{"error": "status must be pending, in_progress, or done"})
				return
			}
			task, getErr := getter.Get(c.Request.Context(), uint(idValue))
			if getErr != nil && !errors.Is(getErr, domain.ErrNotFound) {
				log.Printf("task CSV export lookup failed: %v", getErr)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "could not export tasks"})
				return
			}
			assignee := strings.TrimSpace(c.Query("assignee"))
			if getErr == nil && (status == "" || status == task.Status) && (assignee == "" || assignee == task.AssignedTo) && taskCreatedAtInRange(task.CreatedAt, createdFrom, createdUntil) {
				tasks = []domain.Task{task}
				total = 1
			}
		} else {
			tasks, total, err = h.tasks.List(c.Request.Context(), domain.TaskFilter{
				Status: status, Assignee: c.Query("assignee"), Query: query,
				CreatedFrom: createdFrom, CreatedUntil: createdUntil, Limit: MaxCSVExportRows,
			})
		}
	} else {
		tasks, total, err = h.tasks.List(c.Request.Context(), domain.TaskFilter{
			Status: status, Assignee: c.Query("assignee"), Query: query,
			CreatedFrom: createdFrom, CreatedUntil: createdUntil, Limit: MaxCSVExportRows,
		})
	}
	if errors.Is(err, usecase.ErrTaskInvalidStatus) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "status must be pending, in_progress, or done"})
		return
	}
	if err != nil {
		log.Printf("task CSV export failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not export tasks"})
		return
	}
	if total > MaxCSVExportRows {
		c.JSON(http.StatusConflict, gin.H{"error": "More than 10,000 tasks match. Narrow the status or search filter and export again."})
		return
	}
	if len(tasks) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "No tasks match the current filters."})
		return
	}
	rows := make([][]string, 0, len(tasks))
	for _, task := range tasks {
		rows = append(rows, []string{
			strconv.FormatUint(uint64(task.ID), 10), task.Title, task.Description,
			task.CreatedBy, task.AssignedTo, task.Priority, task.DueDate,
			task.Status, task.CreatedAt.UTC().Format(time.RFC3339), task.UpdatedAt.UTC().Format(time.RFC3339),
		})
	}
	filename := "tasks-" + time.Now().UTC().Format("20060102-150405") + ".csv"
	writeCSVResponse(c, filename, []string{"ID", "Title", "Details", "Given by", "Assigned to", "Priority", "Due date", "Status", "Created at (UTC)", "Updated at (UTC)"}, rows)
}

func writeCSVResponse(c *gin.Context, filename string, headers []string, rows [][]string) {
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Cache-Control", "private, no-store")
	c.Status(http.StatusOK)
	if _, err := c.Writer.Write([]byte{0xef, 0xbb, 0xbf}); err != nil {
		log.Printf("CSV response interrupted before rows were written")
		return
	}
	writer := csv.NewWriter(c.Writer)
	if err := writer.Write(csvSafeRow(headers)); err != nil {
		log.Printf("CSV header write failed: %v", err)
		return
	}
	for _, row := range rows {
		if err := writer.Write(csvSafeRow(row)); err != nil {
			log.Printf("CSV row write failed: %v", err)
			return
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		log.Printf("CSV response failed: %v", err)
	}
}

func csvSafeRow(row []string) []string {
	safe := make([]string, len(row))
	for i, value := range row {
		trimmed := strings.TrimLeftFunc(value, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) })
		if trimmed != "" && strings.ContainsRune("=+-@", rune(trimmed[0])) {
			value = "'" + value
		}
		safe[i] = value
	}
	return safe
}

func (h *Handler) createTask(c *gin.Context) {
	if h.tasks == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "task service is unavailable"})
		return
	}
	var request createTaskRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid task request"})
		return
	}
	assignee := strings.TrimSpace(request.AssignedTo)
	if assignee != "" {
		assignee = resolveTaskAssigneeMention(assignee)
		if assignee == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "choose an IT Team member or IT Team from the list"})
			return
		}
	}
	task, err := h.tasks.Create(c.Request.Context(), usecase.CreateTaskInput{
		Title: request.Title, Description: request.Description, CreatedBy: currentCreatorName(c), AssignedTo: assignee,
		Priority: request.Priority, DueDate: request.DueDate,
	})
	if err != nil {
		writeTaskError(c, err)
		return
	}
	h.recordWorkActivity(c, "task", task.ID, "created", "Task created: "+task.Title)
	c.JSON(http.StatusCreated, gin.H{"task": toTaskView(task)})
}

func (h *Handler) updateTask(c *gin.Context) {
	if h.tasks == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "task service is unavailable"})
		return
	}
	idValue, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || idValue == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid task ID"})
		return
	}
	var request updateTaskRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid task update"})
		return
	}
	if request.AssignedTo != nil && strings.TrimSpace(*request.AssignedTo) != "" {
		resolved := resolveTaskAssigneeMention(*request.AssignedTo)
		if resolved == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "choose an IT Team member or IT Team from the list"})
			return
		}
		request.AssignedTo = &resolved
	}
	var previous domain.Task
	if getter, ok := h.tasks.(taskGetter); ok {
		previous, _ = getter.Get(c.Request.Context(), uint(idValue))
	}
	// Keep task creator attribution immutable after creation; edits should not
	// rewrite which signed-in account originally created the task.
	request.CreatedBy = nil
	task, err := h.tasks.Update(c.Request.Context(), uint(idValue), usecase.UpdateTaskInput{
		Title: request.Title, Description: request.Description, CreatedBy: request.CreatedBy, AssignedTo: request.AssignedTo,
		Priority: request.Priority, DueDate: request.DueDate, Status: request.Status,
	})
	if err != nil {
		if err == domain.ErrNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
			return
		}
		writeTaskError(c, err)
		return
	}
	if previous.ID == 0 {
		h.recordWorkActivity(c, "task", task.ID, "updated", "Task was updated")
	} else {
		if previous.Status != task.Status {
			h.recordWorkActivity(c, "task", task.ID, "status_changed", fmt.Sprintf("Status changed: %s → %s", previous.Status, task.Status))
		}
		if previous.AssignedTo != task.AssignedTo {
			oldAssignee, newAssignee := previous.AssignedTo, task.AssignedTo
			if strings.TrimSpace(oldAssignee) == "" {
				oldAssignee = usecase.DefaultTaskAssignee
			}
			if strings.TrimSpace(newAssignee) == "" {
				newAssignee = usecase.DefaultTaskAssignee
			}
			h.recordWorkActivity(c, "task", task.ID, "assignee_changed", fmt.Sprintf("Assignee changed: %s → %s", oldAssignee, newAssignee))
		}
		if previous.Priority != task.Priority {
			h.recordWorkActivity(c, "task", task.ID, "priority_changed", fmt.Sprintf("Priority changed: %s → %s", previous.Priority, task.Priority))
		}
		if previous.DueDate != task.DueDate {
			oldDue, newDue := previous.DueDate, task.DueDate
			if oldDue == "" {
				oldDue = "No due date"
			}
			if newDue == "" {
				newDue = "No due date"
			}
			h.recordWorkActivity(c, "task", task.ID, "updated", fmt.Sprintf("Due date changed: %s → %s", oldDue, newDue))
		}
		if previous.Title != task.Title || previous.Description != task.Description || previous.CreatedBy != task.CreatedBy {
			h.recordWorkActivity(c, "task", task.ID, "updated", "Task details or Given by were updated")
		}
	}
	c.JSON(http.StatusOK, gin.H{"task": toTaskView(task)})
}

func writeTaskError(c *gin.Context, err error) {
	switch err {
	case usecase.ErrTaskTitleRequired, usecase.ErrTaskTitleTooLong, usecase.ErrTaskDescriptionTooLong,
		usecase.ErrTaskCreatorRequired, usecase.ErrTaskCreatorTooLong,
		usecase.ErrTaskAssigneeTooLong, usecase.ErrTaskInvalidPriority, usecase.ErrTaskInvalidStatus,
		usecase.ErrTaskInvalidDueDate, usecase.ErrTaskNoChanges:
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	default:
		log.Printf("task save failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not save task"})
	}
}
