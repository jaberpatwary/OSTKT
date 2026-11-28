package postgres

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"ticket-system/internal/domain"
)

type serviceRecord struct {
	ID          uint   `gorm:"primaryKey"`
	Name        string `gorm:"not null;uniqueIndex"`
	Category    string `gorm:"index;not null"`
	Description string `gorm:"type:text"`
	Priority    string `gorm:"not null;default:medium"`
	Active      bool   `gorm:"not null;default:true;index"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (serviceRecord) TableName() string { return "service_catalog" }

type articleRecord struct {
	ID        uint   `gorm:"primaryKey"`
	Title     string `gorm:"not null;index"`
	Body      string `gorm:"type:text;not null"`
	Tags      string `gorm:"type:text"`
	Published bool   `gorm:"not null;default:false;index"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (articleRecord) TableName() string { return "knowledge_articles" }

type assetRecord struct {
	ID                   uint   `gorm:"primaryKey"`
	AssetTag             string `gorm:"not null;uniqueIndex"`
	Name                 string `gorm:"not null;index"`
	Category             string `gorm:"index"`
	Manufacturer         string
	Model                string
	SerialNumber         string `gorm:"index"`
	AssignedEmployeeID   string `gorm:"index"`
	AssignedEmployeeName string `gorm:"index"`
	Status               string `gorm:"not null;default:in_stock;index"`
	Notes                string `gorm:"type:text"`
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

func (assetRecord) TableName() string { return "assets" }

type changeRecord struct {
	ID          uint       `gorm:"primaryKey"`
	Title       string     `gorm:"not null;index"`
	Description string     `gorm:"type:text"`
	RequestedBy string     `gorm:"index"`
	Owner       string     `gorm:"index"`
	Risk        string     `gorm:"not null;default:medium;index"`
	Status      string     `gorm:"not null;default:requested;index"`
	WindowStart *time.Time `gorm:"index"`
	WindowEnd   *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (changeRecord) TableName() string { return "change_requests" }

type incidentRecord struct {
	ID              uint   `gorm:"primaryKey"`
	RecordType      string `gorm:"not null;index"`
	Title           string `gorm:"not null;index"`
	Description     string `gorm:"type:text"`
	AffectedService string `gorm:"index"`
	Impact          string `gorm:"not null;default:single_user;index"`
	Owner           string `gorm:"index"`
	RelatedTickets  string `gorm:"type:text"`
	RootCause       string `gorm:"type:text"`
	Workaround      string `gorm:"type:text"`
	Status          string `gorm:"not null;default:open;index"`
	CreatedAt       time.Time
	UpdatedAt       time.Time
	ResolvedAt      *time.Time
}

func (incidentRecord) TableName() string { return "incident_problem_records" }

type inboundEmailRecord struct {
	MessageID    string `gorm:"primaryKey;size:320"`
	Status       string `gorm:"not null;index"`
	TicketNumber string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (inboundEmailRecord) TableName() string { return "inbound_email_events" }

type PlatformRepository struct{ db *gorm.DB }

func NewPlatformRepository(db *gorm.DB) *PlatformRepository { return &PlatformRepository{db: db} }

func (r *PlatformRepository) Migrate() error {
	if err := r.db.AutoMigrate(&serviceRecord{}, &articleRecord{}, &assetRecord{}, &changeRecord{}, &incidentRecord{}, &inboundEmailRecord{}); err != nil {
		return err
	}
	var count int64
	if err := r.db.Model(&serviceRecord{}).Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		defaults := []serviceRecord{
			{Name: "Network and internet", Category: "Network", Description: "Wi-Fi, LAN, VPN, and internet connection support.", Priority: "high", Active: true},
			{Name: "Laptop and desktop support", Category: "Hardware", Description: "Computer faults, peripherals, repair, and device setup.", Priority: "medium", Active: true},
			{Name: "Account and access", Category: "Identity and access", Description: "Account setup, password resets, and application permissions.", Priority: "high", Active: true},
			{Name: "Email and collaboration", Category: "Productivity", Description: "Email, calendar, file sharing, and meeting tool support.", Priority: "medium", Active: true},
			{Name: "Software installation", Category: "Software", Description: "Request approved software installation or troubleshooting.", Priority: "low", Active: true},
		}
		if err := r.db.Clauses(clause.OnConflict{DoNothing: true}).Create(&defaults).Error; err != nil {
			return err
		}
	}
	return nil
}

func (r *PlatformRepository) ClaimInboundEmail(ctx context.Context, messageID string) (bool, error) {
	db := r.db.WithContext(ctx)
	now := time.Now().UTC()
	record := inboundEmailRecord{MessageID: messageID, Status: "processing", CreatedAt: now, UpdatedAt: now}
	result := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&record)
	if result.Error != nil {
		return false, result.Error
	}
	if result.RowsAffected == 1 {
		return true, nil
	}
	stale := db.Model(&inboundEmailRecord{}).
		Where("message_id = ? AND status = ? AND updated_at < ?", messageID, "processing", now.Add(-time.Hour)).
		Updates(map[string]any{"status": "processing", "ticket_number": "", "updated_at": now})
	return stale.RowsAffected == 1, stale.Error
}

func (r *PlatformRepository) CompleteInboundEmail(ctx context.Context, messageID, ticketNumber string) error {
	return r.db.WithContext(ctx).Model(&inboundEmailRecord{}).
		Where("message_id = ? AND status = ?", messageID, "processing").
		Updates(map[string]any{"status": "completed", "ticket_number": ticketNumber}).Error
}

func (r *PlatformRepository) InboundEmailStatus(ctx context.Context, messageID string) (string, bool, error) {
	var row inboundEmailRecord
	err := r.db.WithContext(ctx).First(&row, "message_id = ?", messageID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return row.TicketNumber, row.Status == "completed", nil
}

func (r *PlatformRepository) ReleaseInboundEmail(ctx context.Context, messageID string) error {
	return r.db.WithContext(ctx).Where("message_id = ? AND status = ?", messageID, "processing").Delete(&inboundEmailRecord{}).Error
}

func (r *PlatformRepository) ListServices(ctx context.Context, includeInactive bool) ([]domain.ServiceCatalogItem, error) {
	query := r.db.WithContext(ctx).Model(&serviceRecord{})
	if !includeInactive {
		query = query.Where("active = ?", true)
	}
	var rows []serviceRecord
	if err := query.Order("category ASC, name ASC").Limit(500).Find(&rows).Error; err != nil {
		return nil, err
	}
	items := make([]domain.ServiceCatalogItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, domain.ServiceCatalogItem{ID: row.ID, Name: row.Name, Category: row.Category, Description: row.Description, Priority: row.Priority, Active: row.Active, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt})
	}
	return items, nil
}

func (r *PlatformRepository) SaveService(ctx context.Context, item domain.ServiceCatalogItem) (domain.ServiceCatalogItem, error) {
	row := serviceRecord{ID: item.ID, Name: item.Name, Category: item.Category, Description: item.Description, Priority: item.Priority, Active: item.Active}
	if err := savePlatformRecord(r.db.WithContext(ctx), &row, item.ID); err != nil {
		return domain.ServiceCatalogItem{}, err
	}
	return domain.ServiceCatalogItem{ID: row.ID, Name: row.Name, Category: row.Category, Description: row.Description, Priority: row.Priority, Active: row.Active, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}, nil
}

func (r *PlatformRepository) ListArticles(ctx context.Context, search string, includeDrafts bool) ([]domain.KnowledgeArticle, error) {
	query := r.db.WithContext(ctx).Model(&articleRecord{})
	if !includeDrafts {
		query = query.Where("published = ?", true)
	}
	if search = strings.TrimSpace(search); search != "" {
		like := "%" + strings.ToLower(search) + "%"
		query = query.Where("LOWER(title) LIKE ? OR LOWER(body) LIKE ? OR LOWER(tags) LIKE ?", like, like, like)
	}
	var rows []articleRecord
	if err := query.Order("updated_at DESC, id DESC").Limit(200).Find(&rows).Error; err != nil {
		return nil, err
	}
	items := make([]domain.KnowledgeArticle, 0, len(rows))
	for _, row := range rows {
		items = append(items, domain.KnowledgeArticle{ID: row.ID, Title: row.Title, Body: row.Body, Tags: row.Tags, Published: row.Published, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt})
	}
	return items, nil
}

func (r *PlatformRepository) SaveArticle(ctx context.Context, item domain.KnowledgeArticle) (domain.KnowledgeArticle, error) {
	row := articleRecord{ID: item.ID, Title: item.Title, Body: item.Body, Tags: item.Tags, Published: item.Published}
	if err := savePlatformRecord(r.db.WithContext(ctx), &row, item.ID); err != nil {
		return domain.KnowledgeArticle{}, err
	}
	return domain.KnowledgeArticle{ID: row.ID, Title: row.Title, Body: row.Body, Tags: row.Tags, Published: row.Published, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}, nil
}

func (r *PlatformRepository) ListAssets(ctx context.Context, search string) ([]domain.Asset, error) {
	query := r.db.WithContext(ctx).Model(&assetRecord{})
	if search = strings.TrimSpace(search); search != "" {
		like := "%" + strings.ToLower(search) + "%"
		query = query.Where("LOWER(asset_tag) LIKE ? OR LOWER(name) LIKE ? OR LOWER(category) LIKE ? OR LOWER(serial_number) LIKE ? OR LOWER(assigned_employee_name) LIKE ? OR LOWER(assigned_employee_id) LIKE ?", like, like, like, like, like, like)
	}
	var rows []assetRecord
	if err := query.Order("updated_at DESC, id DESC").Limit(1000).Find(&rows).Error; err != nil {
		return nil, err
	}
	items := make([]domain.Asset, 0, len(rows))
	for _, row := range rows {
		items = append(items, domain.Asset{ID: row.ID, AssetTag: row.AssetTag, Name: row.Name, Category: row.Category, Manufacturer: row.Manufacturer, Model: row.Model, SerialNumber: row.SerialNumber, AssignedEmployeeID: row.AssignedEmployeeID, AssignedEmployeeName: row.AssignedEmployeeName, Status: row.Status, Notes: row.Notes, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt})
	}
	return items, nil
}

func (r *PlatformRepository) SaveAsset(ctx context.Context, item domain.Asset) (domain.Asset, error) {
	row := assetRecord{ID: item.ID, AssetTag: item.AssetTag, Name: item.Name, Category: item.Category, Manufacturer: item.Manufacturer, Model: item.Model, SerialNumber: item.SerialNumber, AssignedEmployeeID: item.AssignedEmployeeID, AssignedEmployeeName: item.AssignedEmployeeName, Status: item.Status, Notes: item.Notes}
	if err := savePlatformRecord(r.db.WithContext(ctx), &row, item.ID); err != nil {
		return domain.Asset{}, err
	}
	return domain.Asset{ID: row.ID, AssetTag: row.AssetTag, Name: row.Name, Category: row.Category, Manufacturer: row.Manufacturer, Model: row.Model, SerialNumber: row.SerialNumber, AssignedEmployeeID: row.AssignedEmployeeID, AssignedEmployeeName: row.AssignedEmployeeName, Status: row.Status, Notes: row.Notes, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}, nil
}

func (r *PlatformRepository) ListChanges(ctx context.Context, status string) ([]domain.ChangeRequest, error) {
	query := r.db.WithContext(ctx).Model(&changeRecord{})
	if status = strings.TrimSpace(status); status != "" {
		query = query.Where("status = ?", status)
	}
	var rows []changeRecord
	if err := query.Order("created_at DESC, id DESC").Limit(1000).Find(&rows).Error; err != nil {
		return nil, err
	}
	items := make([]domain.ChangeRequest, 0, len(rows))
	for _, row := range rows {
		items = append(items, domain.ChangeRequest{ID: row.ID, Title: row.Title, Description: row.Description, RequestedBy: row.RequestedBy, Owner: row.Owner, Risk: row.Risk, Status: row.Status, WindowStart: row.WindowStart, WindowEnd: row.WindowEnd, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt})
	}
	return items, nil
}

func (r *PlatformRepository) SaveChange(ctx context.Context, item domain.ChangeRequest) (domain.ChangeRequest, error) {
	row := changeRecord{ID: item.ID, Title: item.Title, Description: item.Description, RequestedBy: item.RequestedBy, Owner: item.Owner, Risk: item.Risk, Status: item.Status, WindowStart: item.WindowStart, WindowEnd: item.WindowEnd}
	if err := savePlatformRecord(r.db.WithContext(ctx), &row, item.ID); err != nil {
		return domain.ChangeRequest{}, err
	}
	return domain.ChangeRequest{ID: row.ID, Title: row.Title, Description: row.Description, RequestedBy: row.RequestedBy, Owner: row.Owner, Risk: row.Risk, Status: row.Status, WindowStart: row.WindowStart, WindowEnd: row.WindowEnd, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}, nil
}

func (r *PlatformRepository) SetChangeStatus(ctx context.Context, id uint, status string) (domain.ChangeRequest, error) {
	var allowedFrom []string
	switch status {
	case "approved", "rejected":
		allowedFrom = []string{"requested"}
	case "scheduled":
		allowedFrom = []string{"approved"}
	case "completed":
		allowedFrom = []string{"scheduled"}
	case "cancelled":
		allowedFrom = []string{"requested", "approved", "scheduled"}
	default:
		return domain.ChangeRequest{}, domain.ErrInvalidTransition
	}
	db := r.db.WithContext(ctx)
	result := db.Model(&changeRecord{}).Where("id = ? AND status IN ?", id, allowedFrom).Update("status", status)
	if result.Error != nil {
		return domain.ChangeRequest{}, result.Error
	}
	if result.RowsAffected == 0 {
		var count int64
		if err := db.Model(&changeRecord{}).Where("id = ?", id).Count(&count).Error; err != nil {
			return domain.ChangeRequest{}, err
		}
		if count == 0 {
			return domain.ChangeRequest{}, domain.ErrNotFound
		}
		return domain.ChangeRequest{}, domain.ErrInvalidTransition
	}
	var row changeRecord
	if err := db.First(&row, id).Error; err != nil {
		return domain.ChangeRequest{}, err
	}
	return domain.ChangeRequest{ID: row.ID, Title: row.Title, Description: row.Description, RequestedBy: row.RequestedBy, Owner: row.Owner, Risk: row.Risk, Status: row.Status, WindowStart: row.WindowStart, WindowEnd: row.WindowEnd, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}, nil
}

func (r *PlatformRepository) ListIncidents(ctx context.Context, recordType, status string) ([]domain.IncidentRecord, error) {
	query := r.db.WithContext(ctx).Model(&incidentRecord{})
	if recordType != "" {
		query = query.Where("record_type = ?", recordType)
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}
	var rows []incidentRecord
	if err := query.Order("updated_at DESC, id DESC").Limit(1000).Find(&rows).Error; err != nil {
		return nil, err
	}
	items := make([]domain.IncidentRecord, 0, len(rows))
	for _, row := range rows {
		items = append(items, incidentFromRecord(row))
	}
	return items, nil
}

func (r *PlatformRepository) SaveIncident(ctx context.Context, item domain.IncidentRecord) (domain.IncidentRecord, error) {
	row := incidentRecord{ID: item.ID, RecordType: item.RecordType, Title: item.Title, Description: item.Description, AffectedService: item.AffectedService, Impact: item.Impact, Owner: item.Owner, RelatedTickets: item.RelatedTickets, RootCause: item.RootCause, Workaround: item.Workaround, Status: item.Status, ResolvedAt: item.ResolvedAt}
	if err := savePlatformRecord(r.db.WithContext(ctx), &row, item.ID); err != nil {
		return domain.IncidentRecord{}, err
	}
	return incidentFromRecord(row), nil
}

func (r *PlatformRepository) SetIncidentStatus(ctx context.Context, id uint, status string) (domain.IncidentRecord, error) {
	var row incidentRecord
	db := r.db.WithContext(ctx)
	if err := db.First(&row, id).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.IncidentRecord{}, domain.ErrNotFound
	} else if err != nil {
		return domain.IncidentRecord{}, err
	}
	if !validIncidentTransition(row.Status, status) {
		return domain.IncidentRecord{}, domain.ErrInvalidTransition
	}
	updates := map[string]any{"status": status}
	if status == "resolved" {
		now := time.Now().UTC()
		updates["resolved_at"] = &now
	}
	if status == "investigating" && (row.Status == "resolved" || row.Status == "closed") {
		updates["resolved_at"] = nil
	}
	result := db.Model(&incidentRecord{}).Where("id = ? AND status = ?", id, row.Status).Updates(updates)
	if result.Error != nil {
		return domain.IncidentRecord{}, result.Error
	}
	if result.RowsAffected == 0 {
		return domain.IncidentRecord{}, domain.ErrInvalidTransition
	}
	if err := db.First(&row, id).Error; err != nil {
		return domain.IncidentRecord{}, err
	}
	return incidentFromRecord(row), nil
}

func incidentFromRecord(row incidentRecord) domain.IncidentRecord {
	return domain.IncidentRecord{ID: row.ID, RecordType: row.RecordType, Title: row.Title, Description: row.Description, AffectedService: row.AffectedService, Impact: row.Impact, Owner: row.Owner, RelatedTickets: row.RelatedTickets, RootCause: row.RootCause, Workaround: row.Workaround, Status: row.Status, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, ResolvedAt: row.ResolvedAt}
}

func validIncidentTransition(current, next string) bool {
	allowed := map[string][]string{
		"open":          {"investigating", "resolved"},
		"investigating": {"identified", "resolved"},
		"identified":    {"monitoring", "resolved"},
		"monitoring":    {"investigating", "resolved"},
		"resolved":      {"closed", "investigating"},
		"closed":        {"investigating"},
	}
	for _, status := range allowed[current] {
		if status == next {
			return true
		}
	}
	return false
}

func savePlatformRecord[T any](db *gorm.DB, record *T, id uint) error {
	if id == 0 {
		return db.Create(record).Error
	}
	result := db.Model(record).Select("*").Omit("created_at").Updates(record)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return db.First(record, id).Error
}

var _ domain.PlatformRepository = (*PlatformRepository)(nil)
