package usecase

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"ticket-system/internal/domain"
)

const (
	ticketSLANoticeWindow = 4 * time.Hour
	taskNoticeWindow      = 24 * time.Hour
	attentionListLimit    = 500
)

type TicketNotificationSource interface {
	List(ctx context.Context, status string, from, until time.Time, limit int) ([]domain.Ticket, int64, error)
}

type TaskNotificationSource interface {
	List(ctx context.Context, filter domain.TaskFilter) ([]domain.Task, int64, error)
}

type UserNotificationSource interface {
	ListUsers(ctx context.Context) ([]domain.User, error)
}

type NotificationService struct {
	tickets    TicketNotificationSource
	tasks      TaskNotificationSource
	users      UserNotificationSource
	repository domain.NotificationRepository
	location   *time.Location
	clock      func() time.Time
}

func NewNotificationService(tickets TicketNotificationSource, tasks TaskNotificationSource, users UserNotificationSource, repository domain.NotificationRepository, location *time.Location) *NotificationService {
	if location == nil {
		location = time.UTC
	}
	return &NotificationService{
		tickets: tickets, tasks: tasks, users: users, repository: repository,
		location: location, clock: time.Now,
	}
}

func (s *NotificationService) Run(ctx context.Context, interval time.Duration) {
	if interval < time.Minute {
		interval = 5 * time.Minute
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	if err := s.Scan(ctx); err != nil {
		log.Printf("notification scan failed: %v", err)
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.Scan(ctx); err != nil {
				log.Printf("notification scan failed: %v", err)
			}
		}
	}
}

func (s *NotificationService) Scan(ctx context.Context) error {
	users, err := s.users.ListUsers(ctx)
	if err != nil {
		return err
	}
	activeUsers := make([]domain.User, 0, len(users))
	for _, user := range users {
		if user.Active {
			activeUsers = append(activeUsers, user)
		}
	}
	if len(activeUsers) == 0 {
		return nil
	}
	now := s.clock().UTC()
	tickets, _, err := s.tickets.List(ctx, "", time.Time{}, time.Time{}, attentionListLimit)
	if err != nil {
		return err
	}
	for _, ticket := range tickets {
		if strings.EqualFold(strings.TrimSpace(ticket.Status), domain.StatusDone) || ticket.SLAPausedAt != nil {
			continue
		}
		dueAt := ticket.SLADueAt
		if dueAt.IsZero() && !ticket.CreatedAt.IsZero() {
			dueAt = ticket.CreatedAt.Add(24 * time.Hour)
		}
		if dueAt.IsZero() {
			continue
		}
		attention, severity := attentionKind(now, dueAt, ticketSLANoticeWindow)
		if attention == "" {
			continue
		}
		kind := "ticket_" + attention
		title := "SLA due soon · " + ticket.TicketNumber
		if attention == "overdue" {
			title, severity = "SLA overdue · "+ticket.TicketNumber, "urgent"
		}
		message := strings.TrimSpace(ticket.EmployeeName + ": " + ticket.Problem)
		if len([]rune(message)) > 1800 {
			message = string([]rune(message)[:1800])
		}
		input := domain.NotificationInput{
			EventKey: fmt.Sprintf("ticket:%d:%s:%d", ticket.ID, kind, dueAt.Unix()),
			Kind:     kind, Severity: severity, Title: title, Message: message, Link: "/admin",
		}
		if err := s.repository.CreateForUsers(ctx, notificationRecipients(activeUsers, ticket.AssignedTo), input); err != nil {
			return err
		}
	}
	tasks, _, err := s.tasks.List(ctx, domain.TaskFilter{Limit: attentionListLimit})
	if err != nil {
		return err
	}
	for _, task := range tasks {
		if strings.EqualFold(strings.TrimSpace(task.Status), domain.StatusDone) || strings.TrimSpace(task.DueDate) == "" {
			continue
		}
		day, err := time.ParseInLocation("2006-01-02", task.DueDate, s.location)
		if err != nil {
			continue
		}
		dueAt := day.Add(24*time.Hour - time.Nanosecond).UTC()
		attention, severity := attentionKind(now, dueAt, taskNoticeWindow)
		if attention == "" {
			continue
		}
		kind := "task_" + attention
		title := fmt.Sprintf("Task due soon · TSK-%06d", task.ID)
		if attention == "overdue" {
			title, severity = fmt.Sprintf("Task overdue · TSK-%06d", task.ID), "urgent"
		}
		message := strings.TrimSpace(task.Title)
		if task.Description != "" {
			message += ": " + strings.TrimSpace(task.Description)
		}
		if len([]rune(message)) > 1800 {
			message = string([]rune(message)[:1800])
		}
		input := domain.NotificationInput{
			EventKey: fmt.Sprintf("task:%d:%s:%s", task.ID, kind, task.DueDate),
			Kind:     kind, Severity: severity, Title: title, Message: message, Link: "/?view=tasks",
		}
		if err := s.repository.CreateForUsers(ctx, notificationRecipients(activeUsers, task.AssignedTo), input); err != nil {
			return err
		}
	}
	return nil
}

func attentionKind(now, dueAt time.Time, window time.Duration) (kind, severity string) {
	if !dueAt.After(now) {
		return "overdue", "urgent"
	}
	if dueAt.Sub(now) <= window {
		return "due_soon", "warning"
	}
	return "", ""
}

func notificationRecipients(users []domain.User, assignee string) []uint {
	assignee = normalizeAssignedUser(assignee)
	matchedAgent := false
	selected := make(map[uint]struct{}, len(users))
	for _, user := range users {
		if user.Role == RoleAdmin || user.Role == RoleManager {
			selected[user.ID] = struct{}{}
		}
		if user.Role == RoleAgent && assignee != "" && (normalizeAssignedUser(user.DisplayName) == assignee || normalizeAssignedUser(user.Username) == assignee) {
			selected[user.ID] = struct{}{}
			matchedAgent = true
		}
	}
	if !matchedAgent {
		for _, user := range users {
			if user.Role == RoleAgent {
				selected[user.ID] = struct{}{}
			}
		}
	}
	userIDs := make([]uint, 0, len(selected))
	for _, user := range users {
		if _, ok := selected[user.ID]; ok {
			userIDs = append(userIDs, user.ID)
		}
	}
	return userIDs
}

func normalizeAssignedUser(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(value)), " "))
}

func (s *NotificationService) List(ctx context.Context, userID uint, unreadOnly bool, limit int) ([]domain.Notification, int64, error) {
	notifications, err := s.repository.ListForUser(ctx, userID, unreadOnly, limit)
	if err != nil {
		return nil, 0, err
	}
	count, err := s.repository.CountUnread(ctx, userID)
	return notifications, count, err
}

func (s *NotificationService) MarkRead(ctx context.Context, userID, notificationID uint) (bool, error) {
	return s.repository.MarkRead(ctx, userID, notificationID)
}

func (s *NotificationService) MarkAllRead(ctx context.Context, userID uint) error {
	return s.repository.MarkAllRead(ctx, userID)
}
