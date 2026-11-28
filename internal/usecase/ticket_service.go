package usecase

import (
	"context"
	"errors"
	"log"
	"strings"
	"sync"
	"time"

	"ticket-system/internal/domain"
)

const (
	MaxSearchResults = 5
	MaxConflictList  = 10
	MaxTicketLimit   = 10000
)

var (
	ErrInvalidStatus   = errors.New("status must be pending, in_progress, or done")
	ErrEmployeeMissing = errors.New("employee name is required")
	ErrEmployeeUnknown = errors.New("employee not found")
	ErrInvalidFilter   = errors.New("invalid ticket status filter")
	ErrProblemRequired = errors.New("problem description is required")
	ErrProblemTooLong  = errors.New("problem description must be 2000 characters or fewer")
	ErrAssigneeTooLong = errors.New("assignee name must be 100 characters or fewer")
	ErrInvalidPriority = errors.New("priority must be low, medium, high, or urgent")
)

type EmployeeDirectory interface {
	Search(query string, limit int) []domain.Employee
	All() []domain.Employee
	Resolve(name, employeeID string) []domain.Employee
}

type Mailer interface {
	Enabled() bool
	SendTicket(ctx context.Context, ticket domain.Ticket, location *time.Location) error
}

type AmbiguousEmployeeError struct {
	Matches []domain.Employee
}

func (e *AmbiguousEmployeeError) Error() string { return "multiple employees match" }

type CreateTicketInput struct {
	Name       string
	EmployeeID string
	Status     string
	Problem    string
	AssignedTo string
	Priority   string
}

type CompleteTicketResult struct {
	Ticket      domain.Ticket
	AlreadyDone bool
	EmailQueued bool
}

type TicketService struct {
	repository domain.TicketRepository
	employees  EmployeeDirectory
	mailer     Mailer
	location   *time.Location
	emailWG    sync.WaitGroup
}

func NewTicketService(repository domain.TicketRepository, employees EmployeeDirectory, mailer Mailer, location *time.Location) *TicketService {
	if location == nil {
		location = time.UTC
	}
	return &TicketService{repository: repository, employees: employees, mailer: mailer, location: location}
}

func (s *TicketService) SearchEmployees(query string, limit int) []domain.Employee {
	return s.employees.Search(query, limit)
}

func (s *TicketService) ListEmployees() []domain.Employee {
	return s.employees.All()
}

func (s *TicketService) Create(ctx context.Context, input CreateTicketInput) (domain.Ticket, bool, error) {
	status := strings.ToLower(strings.TrimSpace(input.Status))
	if status != domain.StatusDone && status != domain.StatusPending && status != domain.StatusInProgress {
		return domain.Ticket{}, false, ErrInvalidStatus
	}
	if strings.TrimSpace(input.Name) == "" && strings.TrimSpace(input.EmployeeID) == "" {
		return domain.Ticket{}, false, ErrEmployeeMissing
	}
	problem := strings.TrimSpace(input.Problem)
	if problem == "" {
		return domain.Ticket{}, false, ErrProblemRequired
	}
	if len([]rune(problem)) > 2000 {
		return domain.Ticket{}, false, ErrProblemTooLong
	}
	priority := strings.ToLower(strings.TrimSpace(input.Priority))
	if priority == "" {
		priority = inferTicketPriority(problem)
	}
	slaHours, ok := ticketSLAHours(priority)
	if !ok {
		return domain.Ticket{}, false, ErrInvalidPriority
	}
	assignee := strings.TrimSpace(input.AssignedTo)
	if len([]rune(assignee)) > 100 {
		return domain.Ticket{}, false, ErrAssigneeTooLong
	}
	matches := s.employees.Resolve(input.Name, input.EmployeeID)
	switch len(matches) {
	case 0:
		return domain.Ticket{}, false, ErrEmployeeUnknown
	case 1:
		// Continue with the one exact or partial employee match.
	default:
		if len(matches) > MaxConflictList {
			matches = matches[:MaxConflictList]
		}
		return domain.Ticket{}, false, &AmbiguousEmployeeError{Matches: matches}
	}

	employee := matches[0]
	ticket := domain.Ticket{
		EmployeeName: employee.Name,
		EmployeeID:   employee.EmployeeID,
		Department:   employee.Department,
		Problem:      problem,
		AssignedTo:   assignee,
		Priority:     priority,
		SLADueAt:     time.Now().UTC().Add(time.Duration(slaHours) * time.Hour),
		Email:        employee.Email,
		Phone:        employee.Phone,
		Status:       status,
	}
	if err := s.repository.Create(ctx, &ticket); err != nil {
		return domain.Ticket{}, false, err
	}
	queued := status == domain.StatusDone && s.queueEmail(ticket)
	return ticket, queued, nil
}

func ticketSLAHours(priority string) (int, bool) {
	switch strings.ToLower(strings.TrimSpace(priority)) {
	case "urgent":
		return 4, true
	case "high":
		return 8, true
	case "medium":
		return 24, true
	case "low":
		return 72, true
	default:
		return 0, false
	}
}

func inferTicketPriority(problem string) string {
	text := strings.ToLower(problem)
	for _, marker := range []string{
		"security breach", "data breach", "ransomware", "production down", "server down",
		"entire office", "all users", "puro office", "office er sob", "sobai kaj",
		"সার্ভার ডাউন", "পুরো অফিস", "সবাইয়ের কাজ", "ডেটা চুরি", "র‍্যানসমওয়্যার", "নিরাপত্তা সমস্যা",
	} {
		if strings.Contains(text, marker) {
			return "urgent"
		}
	}
	for _, marker := range []string{
		"internet nai", "internet nei", "network down", "system down", "can't work",
		"cannot work", "unable to work", "kaj korte parchi na", "kaj bondho", "pc cholche na",
		"ইন্টারনেট নেই", "নেট নেই", "কম্পিউটার চলছে না", "ল্যাপটপ চলছে না", "কাজ বন্ধ", "কাজ করতে পারছি না",
	} {
		if strings.Contains(text, marker) {
			return "high"
		}
	}
	return "medium"
}

func (s *TicketService) Complete(ctx context.Context, key string) (CompleteTicketResult, error) {
	ticket, err := s.repository.GetByKey(ctx, key)
	if err != nil {
		return CompleteTicketResult{}, err
	}
	if ticket.Status == domain.StatusDone {
		return CompleteTicketResult{Ticket: ticket, AlreadyDone: true}, nil
	}

	updated, err := s.repository.CompletePending(ctx, ticket.ID)
	if err != nil {
		return CompleteTicketResult{}, err
	}
	ticket.Status = domain.StatusDone
	if !updated {
		current, readErr := s.repository.GetByKey(ctx, key)
		if readErr != nil {
			return CompleteTicketResult{}, readErr
		}
		return CompleteTicketResult{Ticket: current, AlreadyDone: true}, nil
	}
	queued := false
	if ticket.EmailSentAt == nil {
		queued = s.queueEmail(ticket)
	}
	return CompleteTicketResult{Ticket: ticket, EmailQueued: queued}, nil
}

func (s *TicketService) SetStatus(ctx context.Context, key, status string) (domain.Ticket, bool, error) {
	status = strings.ToLower(strings.TrimSpace(status))
	if status != domain.StatusPending && status != domain.StatusInProgress && status != domain.StatusDone {
		return domain.Ticket{}, false, ErrInvalidStatus
	}
	if status == domain.StatusDone {
		result, err := s.Complete(ctx, key)
		return result.Ticket, result.EmailQueued, err
	}
	ticket, err := s.repository.GetByKey(ctx, key)
	if err != nil {
		return domain.Ticket{}, false, err
	}
	if ticket.Status == status {
		return ticket, false, nil
	}
	updated, err := s.repository.SetStatus(ctx, ticket.ID, status)
	if err != nil {
		return domain.Ticket{}, false, err
	}
	if !updated {
		return domain.Ticket{}, false, domain.ErrNotFound
	}
	ticket.Status = status
	return ticket, false, nil
}

func (s *TicketService) Get(ctx context.Context, key string) (domain.Ticket, error) {
	return s.repository.GetByKey(ctx, key)
}

func (s *TicketService) Assign(ctx context.Context, key, assignee string) (domain.Ticket, error) {
	assignee = strings.TrimSpace(assignee)
	if len([]rune(assignee)) > 100 {
		return domain.Ticket{}, ErrAssigneeTooLong
	}
	ticket, err := s.repository.GetByKey(ctx, key)
	if err != nil {
		return domain.Ticket{}, err
	}
	updated, err := s.repository.SetAssignee(ctx, ticket.ID, assignee)
	if err != nil {
		return domain.Ticket{}, err
	}
	if !updated {
		return domain.Ticket{}, domain.ErrNotFound
	}
	ticket.AssignedTo = assignee
	return ticket, nil
}

func (s *TicketService) SetPriority(ctx context.Context, key, priority string) (domain.Ticket, error) {
	priority = strings.ToLower(strings.TrimSpace(priority))
	hours, ok := ticketSLAHours(priority)
	if !ok {
		return domain.Ticket{}, ErrInvalidPriority
	}
	ticket, err := s.repository.GetByKey(ctx, key)
	if err != nil {
		return domain.Ticket{}, err
	}
	startedAt := ticket.CreatedAt
	if startedAt.IsZero() {
		startedAt = time.Now().UTC()
	}
	dueAt := startedAt.UTC().Add(time.Duration(hours) * time.Hour)
	updated, err := s.repository.SetPriority(ctx, ticket.ID, priority, dueAt)
	if err != nil {
		return domain.Ticket{}, err
	}
	if !updated {
		return domain.Ticket{}, domain.ErrNotFound
	}
	ticket.Priority = priority
	ticket.SLADueAt = dueAt
	return ticket, nil
}

func (s *TicketService) List(ctx context.Context, status string, from, until time.Time, limit int) ([]domain.Ticket, int64, error) {
	status = strings.ToLower(strings.TrimSpace(status))
	if status != "" && status != domain.StatusDone && status != domain.StatusPending && status != domain.StatusInProgress {
		return nil, 0, ErrInvalidFilter
	}
	if limit <= 0 || limit > MaxTicketLimit {
		limit = 100
	}
	return s.repository.List(ctx, domain.TicketFilter{Status: status, CreatedFrom: from, CreatedUntil: until, Limit: limit})
}

func (s *TicketService) queueEmail(ticket domain.Ticket) bool {
	if strings.TrimSpace(ticket.Email) == "" {
		log.Printf("📭 %s: no employee email address; notification skipped", ticket.TicketNumber)
		return false
	}
	if s.mailer == nil || !s.mailer.Enabled() {
		log.Printf("📭 %s: SMTP is not configured; notification skipped", ticket.TicketNumber)
		return false
	}
	s.emailWG.Add(1)
	go func() {
		defer s.emailWG.Done()
		defer func() {
			if recovered := recover(); recovered != nil {
				log.Printf("email task panic for %s: %v", ticket.TicketNumber, recovered)
			}
		}()
		s.sendTicketEmail(ticket)
	}()
	return true
}

func (s *TicketService) sendTicketEmail(ticket domain.Ticket) {
	const maxAttempts = 3
	var sendErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		sendErr = s.mailer.SendTicket(ctx, ticket, s.location)
		cancel()
		if sendErr == nil {
			break
		}
		log.Printf("email attempt %d/%d failed for %s: %v", attempt, maxAttempts, ticket.TicketNumber, sendErr)
		if attempt < maxAttempts {
			time.Sleep(time.Duration(attempt) * 3 * time.Second)
		}
	}
	if sendErr != nil {
		log.Printf("email delivery failed for %s; ticket remains saved", ticket.TicketNumber)
		return
	}
	if err := s.repository.SetEmailSentAt(context.Background(), ticket.ID, time.Now()); err != nil {
		log.Printf("email_sent_at update failed for %s: %v", ticket.TicketNumber, err)
	}
	log.Printf("email sent for ticket %s", ticket.TicketNumber)
}

func (s *TicketService) WaitForEmails(ctx context.Context) {
	done := make(chan struct{})
	go func() {
		s.emailWG.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
		log.Printf("shutdown deadline reached before email tasks finished")
	}
}
