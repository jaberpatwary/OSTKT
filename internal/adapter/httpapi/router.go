package httpapi

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

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
	Assign(ctx context.Context, key, assignee string) (domain.Ticket, error)
	Get(ctx context.Context, key string) (domain.Ticket, error)
	List(ctx context.Context, status string, from, until time.Time, limit int) ([]domain.Ticket, int64, error)
}

type TaskUseCase interface {
	Create(ctx context.Context, input usecase.CreateTaskInput) (domain.Task, error)
	List(ctx context.Context, filter domain.TaskFilter) ([]domain.Task, int64, error)
	Update(ctx context.Context, id uint, input usecase.UpdateTaskInput) (domain.Task, error)
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
	return taskView{ID: task.ID, Title: task.Title, Description: task.Description, CreatedBy: task.CreatedBy, AssignedTo: task.AssignedTo, Priority: task.Priority, DueDate: task.DueDate, Status: task.Status, CreatedAt: task.CreatedAt, UpdatedAt: task.UpdatedAt}
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
	tasks          TaskUseCase
	assistant      *groqapi.Client
	assistantMu    sync.Mutex
	assistantRates map[string]assistantRateEntry
}

type assistantRateEntry struct {
	window time.Time
	count  int
}

var assistantGluedAssign = regexp.MustCompile(`(?i)\bassing([[:alpha:]])`)
var assistantMisspelledAssign = regexp.MustCompile(`(?i)\bassing\b`)

func normalizeAssistantBanglish(text string) string {
	text = assistantGluedAssign.ReplaceAllString(text, "assign $1")
	return assistantMisspelledAssign.ReplaceAllString(text, "assign")
}

func NewRouter(tickets TicketUseCase, healthCheck func(context.Context) error, cfg config.Config, taskServices ...TaskUseCase) *gin.Engine {
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
	if len(taskServices) > 0 {
		handler.tasks = taskServices[0]
	}
	site := router.Group("/")
	if cfg.AppUser != "" && cfg.AppPassword != "" {
		site.Use(gin.BasicAuth(gin.Accounts{cfg.AppUser: cfg.AppPassword}))
		log.Println("🔒 Basic Auth enabled")
	}
	site.Use(func(c *gin.Context) {
		path := c.Request.URL.Path
		if path == "/" || path == "/dashboard" || path == "/admin" || path == "/admin/tasks" {
			c.Header("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
			c.Header("Pragma", "no-cache")
		}
		c.Next()
	})
	site.StaticFile("/", "./static/index.html")
	site.StaticFile("/dashboard", "./static/index.html")
	site.StaticFile("/admin", "./static/admin.html")
	site.StaticFile("/admin/tasks", "./static/admin-tasks.html")
	site.StaticFile("/theme.css", "./static/theme.css")
	site.StaticFile("/premium.css", "./static/premium.css")
	site.StaticFile("/theme.js", "./static/theme.js")

	api := site.Group("/api")
	api.GET("/search", handler.searchEmployees)
	api.GET("/assistant/status", handler.assistantStatus)
	api.POST("/assistant", handler.assistantChat)
	api.POST("/assistant/tasks", handler.assistantTaskChat)
	api.POST("/assistant/transcribe", handler.assistantTranscribe)
	api.GET("/tickets", handler.listTickets)
	api.POST("/tickets", handler.createTicket)
	api.GET("/tickets/:id", handler.getTicket)
	api.POST("/tickets/:id/done", handler.markDone)
	api.PUT("/tickets/:id/assignee", handler.assignTicket)
	api.GET("/tasks", handler.listTasks)
	api.POST("/tasks", handler.createTask)
	api.PATCH("/tasks/:id", handler.updateTask)

	router.NoRoute(func(c *gin.Context) {
		c.JSON(http.StatusNotFound, gin.H{"error": "page not found"})
	})
	return router
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
	match := regexp.MustCompile(`(?i)(?:assign(?:ed|ee)?|assing|give)\s*(?:to|by)?\s*(.+)$`).FindStringSubmatch(text)
	if len(match) < 2 {
		if m := regexp.MustCompile(`(?i)([\pL.]+(?:\s+[\pL.]+){0,3})\s+ke\s+dao\s*$`).FindStringSubmatch(text); len(m) > 1 {
			return resolveAssistantAssignee(m[1])
		}
		return ""
	}
	clause := strings.ToLower(match[1])
	for _, assignee := range assistantAssignees {
		if strings.Contains(clause, strings.ToLower(assignee)) {
			return assignee
		}
	}
	if strings.Contains(clause, "jaber") {
		return resolveAssistantAssignee("jaber")
	}
	return ""
}

func fallbackTaskDetails(messages []groqapi.Message, selectedAssignee string) groqapi.TaskDetails {
	var userText strings.Builder
	createdBy, description := "", ""
	askedForCreator, askedForDescription := false, false
	creatorOnlyAnswer := false
	for _, message := range messages {
		content := strings.TrimSpace(message.Content)
		lower := strings.ToLower(content)
		if message.Role == "assistant" {
			askedForCreator = strings.Contains(lower, "who gave") || strings.Contains(lower, "given by")
			askedForDescription = strings.Contains(lower, "what needs to be done") || strings.Contains(lower, "task-এ কী কাজ")
			if askedForDescription {
				creatorOnlyAnswer = false
			}
			continue
		}
		if message.Role != "user" {
			continue
		}
		userText.WriteByte(' ')
		userText.WriteString(content)
		if m := regexp.MustCompile(`(?i)(?:given\s*by|request(?:ed)?\s*by)\s*[:=]?\s*([\pL.]+(?:\s+[\pL.]+){0,3})`).FindStringSubmatch(content); len(m) > 1 {
			createdBy = strings.TrimSpace(m[1])
		} else if askedForCreator && !strings.Contains(lower, "assign") {
			createdBy = content
			creatorOnlyAnswer = true
		}
		if strings.Contains(lower, "task details:") {
			description = strings.TrimSpace(content[strings.Index(lower, "task details:")+13:])
			creatorOnlyAnswer = false
		} else if askedForDescription {
			description = content
			creatorOnlyAnswer = false
		}
	}
	all := userText.String()
	if description == "" && !creatorOnlyAnswer {
		description = regexp.MustCompile(`(?i)given\s*by\s*[:=]?\s*[\pL.]+(?:\s+[\pL.]+){0,3}[,.]?`).ReplaceAllString(all, " ")
		description = regexp.MustCompile(`(?i)assign(?:ed)?\s*(?:to|by)?\s*[\pL.]+(?:\s+[\pL.]+){0,5}`).ReplaceAllString(description, " ")
	}
	description = strings.TrimSpace(strings.Trim(description, " ,:;.-"))
	assignee := resolveAssistantAssignee(selectedAssignee)
	if assignee == "" {
		assignee = fallbackAssigneeFromText(all)
	}
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
	var details groqapi.TaskDetails
	if h.assistant.Enabled() {
		var err error
		details, err = h.assistant.ExtractTask(c.Request.Context(), request.Messages, assistantAssignees)
		if err != nil {
			log.Printf("task assistant extraction failed; using programming fallback: %v", err)
		}
	}
	fallback := fallbackTaskDetails(request.Messages, request.SelectedAssignee)
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
	if selected := strings.TrimSpace(request.SelectedAssignee); selected != "" {
		details.Assignee = resolveAssistantAssignee(selected)
		if details.Assignee == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "selected assignee is invalid"})
			return
		}
	} else if details.Assignee != "" {
		details.Assignee = resolveAssistantAssignee(details.Assignee)
	}
	if details.CreatedBy == "" {
		c.JSON(http.StatusOK, gin.H{"reply": "প্রথমে বলুন—এই task-টি কে দিয়েছেন? Given by নামটি বলুন।"})
		return
	}
	if len([]rune(details.CreatedBy)) > 100 {
		c.JSON(http.StatusOK, gin.H{"reply": "Given by নামটি ১০০ অক্ষরের মধ্যে ছোট করে বলুন।"})
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
		c.JSON(http.StatusOK, gin.H{"reply": "এবার IT Team থেকে কাকে assign করব? নামটি বলুন বা তালিকা থেকে বেছে নিন।", "needs_assignee": true})
		return
	}
	if len([]rune(details.Title)) > 160 {
		c.JSON(http.StatusOK, gin.H{"reply": "Task-এর শিরোনামটি ১৬০ অক্ষরের মধ্যে ছোট করে বলুন।"})
		return
	}
	task, err := h.tasks.Create(c.Request.Context(), usecase.CreateTaskInput{
		Title: details.Title, Description: details.Description, CreatedBy: details.CreatedBy,
		AssignedTo: details.Assignee, Priority: details.Priority, DueDate: details.DueDate,
	})
	if err != nil {
		log.Printf("task assistant save failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not create task"})
		return
	}
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
	tickets, total, err := h.tickets.List(c.Request.Context(), status, from, until, limit)
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
	limit, err := strconv.Atoi(c.DefaultQuery("limit", "100"))
	if err != nil || limit <= 0 || limit > usecase.MaxTaskLimit {
		limit = 100
	}
	tasks, total, err := h.tasks.List(c.Request.Context(), domain.TaskFilter{
		Status: c.Query("status"), Assignee: c.Query("assignee"), Query: c.Query("q"), Limit: limit,
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
	c.JSON(http.StatusOK, gin.H{"tasks": views, "count": len(views), "total": total})
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
		assignee = resolveAssistantAssignee(assignee)
		if assignee == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "choose an IT Team member from the list"})
			return
		}
	}
	task, err := h.tasks.Create(c.Request.Context(), usecase.CreateTaskInput{
		Title: request.Title, Description: request.Description, CreatedBy: request.CreatedBy, AssignedTo: assignee,
		Priority: request.Priority, DueDate: request.DueDate,
	})
	if err != nil {
		writeTaskError(c, err)
		return
	}
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
		resolved := resolveAssistantAssignee(*request.AssignedTo)
		if resolved == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "choose an IT Team member from the list"})
			return
		}
		request.AssignedTo = &resolved
	}
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
