package usecase

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	"ticket-system/internal/domain"
)

var ErrInvalidPlatformInput = errors.New("invalid operations workspace input")

type PlatformService struct{ repository domain.PlatformRepository }

func NewPlatformService(repository domain.PlatformRepository) *PlatformService {
	return &PlatformService{repository: repository}
}

func (s *PlatformService) Services(ctx context.Context, includeInactive bool) ([]domain.ServiceCatalogItem, error) {
	return s.repository.ListServices(ctx, includeInactive)
}

func (s *PlatformService) SaveService(ctx context.Context, item domain.ServiceCatalogItem) (domain.ServiceCatalogItem, error) {
	item.Name = cleanField(item.Name)
	item.Category = cleanField(item.Category)
	item.Description = strings.TrimSpace(item.Description)
	item.Priority = strings.ToLower(strings.TrimSpace(item.Priority))
	if item.Priority == "" {
		item.Priority = "medium"
	}
	if item.Category == "" {
		item.Category = "General"
	}
	if item.Priority != "low" && item.Priority != "medium" && item.Priority != "high" && item.Priority != "urgent" {
		return domain.ServiceCatalogItem{}, ErrInvalidPlatformInput
	}
	if item.Name == "" || runeCount(item.Name) > 120 || runeCount(item.Category) > 80 || runeCount(item.Description) > 1000 {
		return domain.ServiceCatalogItem{}, ErrInvalidPlatformInput
	}
	return s.repository.SaveService(ctx, item)
}

func (s *PlatformService) Articles(ctx context.Context, search string, canSeeDrafts bool) ([]domain.KnowledgeArticle, error) {
	if runeCount(search) > 200 {
		search = string([]rune(search)[:200])
	}
	return s.repository.ListArticles(ctx, search, canSeeDrafts)
}

func (s *PlatformService) SaveArticle(ctx context.Context, item domain.KnowledgeArticle) (domain.KnowledgeArticle, error) {
	item.Title = cleanField(item.Title)
	item.Body = strings.TrimSpace(item.Body)
	item.Tags = strings.TrimSpace(item.Tags)
	if item.Title == "" || item.Body == "" || runeCount(item.Title) > 180 || runeCount(item.Body) > 30000 || runeCount(item.Tags) > 500 {
		return domain.KnowledgeArticle{}, ErrInvalidPlatformInput
	}
	return s.repository.SaveArticle(ctx, item)
}

func (s *PlatformService) Assets(ctx context.Context, search string) ([]domain.Asset, error) {
	if runeCount(search) > 200 {
		search = string([]rune(search)[:200])
	}
	return s.repository.ListAssets(ctx, search)
}

func (s *PlatformService) SaveAsset(ctx context.Context, item domain.Asset) (domain.Asset, error) {
	item.AssetTag = strings.ToUpper(cleanField(item.AssetTag))
	item.Name = cleanField(item.Name)
	item.Category = cleanField(item.Category)
	item.Manufacturer = cleanField(item.Manufacturer)
	item.Model = cleanField(item.Model)
	item.SerialNumber = cleanField(item.SerialNumber)
	item.AssignedEmployeeID = cleanField(item.AssignedEmployeeID)
	item.AssignedEmployeeName = cleanField(item.AssignedEmployeeName)
	item.Status = strings.ToLower(cleanField(item.Status))
	item.Notes = strings.TrimSpace(item.Notes)
	if item.Status == "" {
		item.Status = "in_stock"
	}
	switch item.Status {
	case "in_stock", "assigned", "maintenance", "retired", "lost":
	default:
		return domain.Asset{}, ErrInvalidPlatformInput
	}
	if item.AssetTag == "" || item.Name == "" || runeCount(item.AssetTag) > 60 || runeCount(item.Name) > 160 || runeCount(item.Category) > 100 || runeCount(item.Manufacturer) > 100 || runeCount(item.Model) > 100 || runeCount(item.SerialNumber) > 120 || runeCount(item.AssignedEmployeeID) > 80 || runeCount(item.AssignedEmployeeName) > 160 || runeCount(item.Notes) > 2000 {
		return domain.Asset{}, ErrInvalidPlatformInput
	}
	if item.Status == "assigned" && item.AssignedEmployeeName == "" {
		return domain.Asset{}, ErrInvalidPlatformInput
	}
	if item.Status != "assigned" {
		item.AssignedEmployeeID, item.AssignedEmployeeName = "", ""
	}
	return s.repository.SaveAsset(ctx, item)
}

func (s *PlatformService) Changes(ctx context.Context, status string) ([]domain.ChangeRequest, error) {
	status = strings.ToLower(strings.TrimSpace(status))
	if status != "" && !validChangeStatus(status) {
		return nil, ErrInvalidPlatformInput
	}
	return s.repository.ListChanges(ctx, status)
}

func (s *PlatformService) SaveChange(ctx context.Context, item domain.ChangeRequest, creating bool) (domain.ChangeRequest, error) {
	item.Title = cleanField(item.Title)
	item.Description = strings.TrimSpace(item.Description)
	item.RequestedBy = cleanField(item.RequestedBy)
	item.Owner = cleanField(item.Owner)
	item.Risk = strings.ToLower(cleanField(item.Risk))
	item.Status = strings.ToLower(cleanField(item.Status))
	if item.Risk == "" {
		item.Risk = "medium"
	}
	if creating {
		item.Status = "requested"
	}
	if item.Status == "" {
		item.Status = "requested"
	}
	if item.Title == "" || item.RequestedBy == "" || runeCount(item.Title) > 180 || runeCount(item.Description) > 5000 || runeCount(item.RequestedBy) > 160 || runeCount(item.Owner) > 160 {
		return domain.ChangeRequest{}, ErrInvalidPlatformInput
	}
	if item.Risk != "low" && item.Risk != "medium" && item.Risk != "high" {
		return domain.ChangeRequest{}, ErrInvalidPlatformInput
	}
	if !validChangeStatus(item.Status) {
		return domain.ChangeRequest{}, ErrInvalidPlatformInput
	}
	if item.WindowStart != nil && item.WindowEnd != nil && !item.WindowEnd.After(*item.WindowStart) {
		return domain.ChangeRequest{}, ErrInvalidPlatformInput
	}
	return s.repository.SaveChange(ctx, item)
}

func (s *PlatformService) SetChangeStatus(ctx context.Context, id uint, status string) (domain.ChangeRequest, error) {
	status = strings.ToLower(strings.TrimSpace(status))
	if !validChangeStatus(status) || status == "requested" {
		return domain.ChangeRequest{}, ErrInvalidPlatformInput
	}
	return s.repository.SetChangeStatus(ctx, id, status)
}

func (s *PlatformService) Incidents(ctx context.Context, recordType, status string) ([]domain.IncidentRecord, error) {
	recordType, status = strings.ToLower(strings.TrimSpace(recordType)), strings.ToLower(strings.TrimSpace(status))
	if recordType != "" && recordType != "incident" && recordType != "problem" {
		return nil, ErrInvalidPlatformInput
	}
	if status != "" && !validIncidentStatus(status) {
		return nil, ErrInvalidPlatformInput
	}
	return s.repository.ListIncidents(ctx, recordType, status)
}

func (s *PlatformService) SaveIncident(ctx context.Context, item domain.IncidentRecord, creating bool) (domain.IncidentRecord, error) {
	item.RecordType = strings.ToLower(cleanField(item.RecordType))
	item.Title = cleanField(item.Title)
	item.Description = strings.TrimSpace(item.Description)
	item.AffectedService = cleanField(item.AffectedService)
	item.Impact = strings.ToLower(cleanField(item.Impact))
	item.Owner = cleanField(item.Owner)
	item.RelatedTickets = strings.TrimSpace(item.RelatedTickets)
	item.RootCause = strings.TrimSpace(item.RootCause)
	item.Workaround = strings.TrimSpace(item.Workaround)
	if item.RecordType == "" {
		item.RecordType = "incident"
	}
	if item.Impact == "" {
		item.Impact = "single_user"
	}
	if creating {
		item.Status = "open"
	}
	if item.Status == "" {
		item.Status = "open"
	}
	if (item.RecordType != "incident" && item.RecordType != "problem") || !validIncidentStatus(item.Status) {
		return domain.IncidentRecord{}, ErrInvalidPlatformInput
	}
	switch item.Impact {
	case "single_user", "department", "organization", "critical_service":
	default:
		return domain.IncidentRecord{}, ErrInvalidPlatformInput
	}
	if item.Title == "" || runeCount(item.Title) > 180 || runeCount(item.Description) > 5000 || runeCount(item.AffectedService) > 120 || runeCount(item.Owner) > 160 || runeCount(item.RelatedTickets) > 1000 || runeCount(item.RootCause) > 4000 || runeCount(item.Workaround) > 4000 {
		return domain.IncidentRecord{}, ErrInvalidPlatformInput
	}
	return s.repository.SaveIncident(ctx, item)
}

func (s *PlatformService) SetIncidentStatus(ctx context.Context, id uint, status string) (domain.IncidentRecord, error) {
	status = strings.ToLower(strings.TrimSpace(status))
	if !validIncidentStatus(status) {
		return domain.IncidentRecord{}, ErrInvalidPlatformInput
	}
	return s.repository.SetIncidentStatus(ctx, id, status)
}

func (s *PlatformService) ClaimInboundEmail(ctx context.Context, messageID string) (bool, error) {
	if strings.TrimSpace(messageID) == "" || runeCount(messageID) > 320 {
		return false, ErrInvalidPlatformInput
	}
	return s.repository.ClaimInboundEmail(ctx, strings.TrimSpace(messageID))
}

func (s *PlatformService) InboundEmailStatus(ctx context.Context, messageID string) (string, bool, error) {
	return s.repository.InboundEmailStatus(ctx, strings.TrimSpace(messageID))
}

func (s *PlatformService) CompleteInboundEmail(ctx context.Context, messageID, ticketNumber string) error {
	return s.repository.CompleteInboundEmail(ctx, strings.TrimSpace(messageID), strings.TrimSpace(ticketNumber))
}

func (s *PlatformService) ReleaseInboundEmail(ctx context.Context, messageID string) error {
	return s.repository.ReleaseInboundEmail(ctx, strings.TrimSpace(messageID))
}

func validChangeStatus(status string) bool {
	switch status {
	case "requested", "approved", "rejected", "scheduled", "completed", "cancelled":
		return true
	default:
		return false
	}
}

func validIncidentStatus(status string) bool {
	switch status {
	case "open", "investigating", "identified", "monitoring", "resolved", "closed":
		return true
	default:
		return false
	}
}

func cleanField(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
}
func runeCount(value string) int { return utf8.RuneCountInString(value) }
