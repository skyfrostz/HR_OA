package model

import (
	"encoding/json"
	"time"
)

const (
	RequestInReview = "IN_REVIEW"
	Approved        = "APPROVED"
	Rejected        = "REJECTED"
	TaskPending     = "PENDING"

	ActionSubmit   = "SUBMIT"
	ActionApprove  = "APPROVE"
	ActionReject   = "REJECT"
	ActionTransfer = "TRANSFER"
	ActionHandoff  = "HANDOFF"

	TaskAssigned    = "TASK_ASSIGNED"
	RequestApproved = "REQUEST_APPROVED"
	RequestRejected = "REQUEST_REJECTED"
)

// Workflow tables retain submitted records and history indefinitely in M2.
// The application exposes no update or delete operation for the snapshots.
type ApprovalRequest struct {
	LeaveEmployeeID  *uint             `gorm:"index"`
	LeaveCalculation *LeaveCalculation `gorm:"serializer:json;type:json"`
	RuleSnapshot     *ApprovalRule     `gorm:"serializer:json;type:json"`
	RouteOverrides   map[int]RouteStep `gorm:"serializer:json;type:json"`

	ID                uint            `gorm:"primaryKey"`
	Type              string          `gorm:"size:16;not null"`
	Title             string          `gorm:"size:100;not null"`
	Status            string          `gorm:"size:16;not null"`
	ApplicantID       uint            `gorm:"not null;index"`
	ApplicantName     string          `gorm:"size:255;not null"`
	TemplateVersion   int             `gorm:"not null"`
	Form              json.RawMessage `gorm:"type:json;not null"`
	Route             json.RawMessage `gorm:"type:json;not null"`
	PreviousRequestID *uint
	CurrentStep       *int
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

func (ApprovalRequest) TableName() string { return "oa_requests" }

type ApprovalTask struct {
	ID           uint   `gorm:"primaryKey"`
	RequestID    uint   `gorm:"not null;uniqueIndex:ux_oa_task_step,priority:1"`
	Step         int    `gorm:"not null;uniqueIndex:ux_oa_task_step,priority:2"`
	StepName     string `gorm:"size:100;not null"`
	AssigneeID   uint   `gorm:"not null;index:ix_oa_pending,priority:1"`
	AssigneeName string `gorm:"size:255;not null"`
	Status       string `gorm:"size:16;not null;index:ix_oa_pending,priority:2"`
	Version      int    `gorm:"not null"`
	CreatedAt    time.Time
	CompletedAt  *time.Time
}

func (ApprovalTask) TableName() string { return "oa_tasks" }

type ApprovalAction struct {
	RouteStep          *int
	PreviousAssigneeID *uint
	ID                 uint `gorm:"primaryKey"`
	RequestID          uint `gorm:"not null;index"`
	TaskID             *uint
	ActorID            uint   `gorm:"not null;index"`
	ActorName          string `gorm:"size:255;not null"`
	Action             string `gorm:"size:16;not null"`
	Comment            string `gorm:"type:text;not null"`
	TargetUserID       *uint
	TargetName         *string `gorm:"size:255"`
	CreatedAt          time.Time
}

func (ApprovalAction) TableName() string { return "oa_request_actions" }

type WorkflowIntent struct {
	ID           uint   `gorm:"primaryKey"`
	ActorID      uint   `gorm:"not null;uniqueIndex:ux_oa_intent,priority:1"`
	Route        string `gorm:"size:96;not null;uniqueIndex:ux_oa_intent,priority:2"`
	IntentKey    string `gorm:"type:varbinary(128);not null;uniqueIndex:ux_oa_intent,priority:3"`
	RequestHash  string `gorm:"type:char(64);not null"`
	ResultStatus int    `gorm:"not null"`
	Result       []byte `gorm:"type:mediumblob"`
	CreatedAt    time.Time
}

func (WorkflowIntent) TableName() string { return "oa_intents" }

type NotificationEvent struct {
	ID              uint  `gorm:"primaryKey"`
	ActionID        *uint `gorm:"uniqueIndex"`
	RequestID       uint  `gorm:"not null"`
	ContractID      *uint
	ContractVersion int        `gorm:"not null;default:0"`
	ReminderDays    int        `gorm:"not null;default:0"`
	ScheduleKey     *string    `gorm:"size:100;uniqueIndex"`
	RecipientID     uint       `gorm:"not null"`
	Kind            string     `gorm:"size:32;not null"`
	DeliveryMode    string     `gorm:"size:16;not null;default:mock;index:ix_oa_outbox_dispatch,priority:1"`
	DeliveryStatus  string     `gorm:"size:16;not null;default:PENDING;index:ix_oa_outbox_dispatch,priority:2"`
	Attempts        uint       `gorm:"not null"`
	AvailableAt     time.Time  `gorm:"not null;index:ix_oa_outbox_due,priority:2;index:ix_oa_outbox_dispatch,priority:3"`
	DeliveredAt     *time.Time `gorm:"index:ix_oa_outbox_due,priority:1"`
	CreatedAt       time.Time
}

func (NotificationEvent) TableName() string { return "oa_notification_outbox" }

type MockNotification struct {
	ContractID   *uint
	DeliveryMode string `gorm:"size:16;not null;default:mock"`
	ID           uint   `gorm:"primaryKey"`
	EventID      uint   `gorm:"not null;uniqueIndex"`
	RequestID    uint   `gorm:"not null"`
	RecipientID  uint   `gorm:"not null;index"`
	Kind         string `gorm:"size:32;not null"`
	Title        string `gorm:"size:100;not null"`
	CreatedAt    time.Time
}

func (MockNotification) TableName() string { return "oa_mock_notifications" }
