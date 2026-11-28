package domain

import (
	"context"
	"time"
)

type ServiceCatalogItem struct {
	ID          uint      `json:"id"`
	Name        string    `json:"name"`
	Category    string    `json:"category"`
	Description string    `json:"description"`
	Priority    string    `json:"priority"`
	Active      bool      `json:"active"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type KnowledgeArticle struct {
	ID        uint      `json:"id"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	Tags      string    `json:"tags"`
	Published bool      `json:"published"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Asset struct {
	ID                   uint      `json:"id"`
	AssetTag             string    `json:"asset_tag"`
	Name                 string    `json:"name"`
	Category             string    `json:"category"`
	Manufacturer         string    `json:"manufacturer"`
	Model                string    `json:"model"`
	SerialNumber         string    `json:"serial_number"`
	AssignedEmployeeID   string    `json:"assigned_employee_id"`
	AssignedEmployeeName string    `json:"assigned_employee_name"`
	Status               string    `json:"status"`
	Notes                string    `json:"notes"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

type ChangeRequest struct {
	ID          uint       `json:"id"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	RequestedBy string     `json:"requested_by"`
	Owner       string     `json:"owner"`
	Risk        string     `json:"risk"`
	Status      string     `json:"status"`
	WindowStart *time.Time `json:"window_start,omitempty"`
	WindowEnd   *time.Time `json:"window_end,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type IncidentRecord struct {
	ID              uint       `json:"id"`
	RecordType      string     `json:"record_type"`
	Title           string     `json:"title"`
	Description     string     `json:"description"`
	AffectedService string     `json:"affected_service"`
	Impact          string     `json:"impact"`
	Owner           string     `json:"owner"`
	RelatedTickets  string     `json:"related_tickets"`
	RootCause       string     `json:"root_cause"`
	Workaround      string     `json:"workaround"`
	Status          string     `json:"status"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	ResolvedAt      *time.Time `json:"resolved_at,omitempty"`
}

type PlatformRepository interface {
	Migrate() error
	ListServices(context.Context, bool) ([]ServiceCatalogItem, error)
	SaveService(context.Context, ServiceCatalogItem) (ServiceCatalogItem, error)
	ListArticles(context.Context, string, bool) ([]KnowledgeArticle, error)
	SaveArticle(context.Context, KnowledgeArticle) (KnowledgeArticle, error)
	ListAssets(context.Context, string) ([]Asset, error)
	SaveAsset(context.Context, Asset) (Asset, error)
	ListChanges(context.Context, string) ([]ChangeRequest, error)
	SaveChange(context.Context, ChangeRequest) (ChangeRequest, error)
	SetChangeStatus(context.Context, uint, string) (ChangeRequest, error)
	ListIncidents(context.Context, string, string) ([]IncidentRecord, error)
	SaveIncident(context.Context, IncidentRecord) (IncidentRecord, error)
	SetIncidentStatus(context.Context, uint, string) (IncidentRecord, error)
	ClaimInboundEmail(context.Context, string) (bool, error)
	InboundEmailStatus(context.Context, string) (string, bool, error)
	CompleteInboundEmail(context.Context, string, string) error
	ReleaseInboundEmail(context.Context, string) error
}
