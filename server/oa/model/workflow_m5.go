package model

import "time"

const (
	Withdrawn      = "WITHDRAWN"
	Cancelled      = "CANCELLED"
	ActionWithdraw = "WITHDRAW"
	ActionCancel   = "CANCEL"
	ActionRemind   = "REMIND"
	TaskReminder   = "TASK_REMINDER"
)

type ApprovalRuleStep struct {
	Name       string `json:"name"`
	AssigneeID uint   `json:"assigneeId"`
}
type ApprovalRule struct {
	ID             uint               `gorm:"primaryKey" json:"id"`
	Version        int                `gorm:"not null" json:"version"`
	Name           string             `gorm:"size:100;not null" json:"name"`
	Type           string             `gorm:"size:16;not null;index:ix_oa_rule_match,priority:1" json:"type"`
	Enabled        bool               `gorm:"not null;index:ix_oa_rule_match,priority:2" json:"enabled"`
	DepartmentID   *uint              `json:"departmentId"`
	MinAmountCents *int64             `json:"minAmountCents"`
	MaxAmountCents *int64             `json:"maxAmountCents"`
	Priority       int                `gorm:"not null;index:ix_oa_rule_match,priority:3" json:"priority"`
	Steps          []ApprovalRuleStep `gorm:"serializer:json;type:json;not null" json:"steps"`
}

func (ApprovalRule) TableName() string { return "oa_approval_rules" }

type ApprovalRuleInput struct {
	Name           string             `json:"name"`
	Type           string             `json:"type"`
	Enabled        *bool              `json:"enabled"`
	DepartmentID   *uint              `json:"departmentId"`
	MinAmountCents *int64             `json:"minAmountCents"`
	MaxAmountCents *int64             `json:"maxAmountCents"`
	Priority       int                `json:"priority"`
	Steps          []ApprovalRuleStep `json:"steps"`
	Version        int                `json:"version"`
}
type RuleReference struct {
	ID      uint   `json:"id"`
	Version int    `json:"version"`
	Name    string `json:"name"`
}
type RequestActionInput struct {
	Action string `json:"action"`
	Reason string `json:"reason"`
}
type ApprovedLeave struct {
	ID            uint      `json:"id"`
	ApplicantID   uint      `json:"applicantId"`
	ApplicantName string    `json:"applicantName"`
	LeaveTypeName string    `json:"leaveTypeName"`
	StartDate     string    `json:"startDate"`
	EndDate       string    `json:"endDate"`
	TotalHalfDays int       `json:"totalHalfDays"`
	UpdatedAt     time.Time `json:"updatedAt"`
}
type WorkflowException struct {
	TaskID      uint      `json:"taskId"`
	RequestID   uint      `json:"requestId"`
	RequestType string    `json:"requestType"`
	Step        int       `json:"step"`
	StepName    string    `json:"stepName"`
	AssigneeID  uint      `json:"assigneeId"`
	Reason      string    `json:"reason"`
	Version     int       `json:"version"`
	CreatedAt   time.Time `json:"createdAt"`
}
type ExceptionHandoffInput struct {
	Step         int    `json:"step"`
	Version      int    `json:"version"`
	TargetUserID uint   `json:"targetUserId"`
	Reason       string `json:"reason"`
}
type ExceptionHandoffResult struct {
	TaskID     uint `json:"taskId"`
	RequestID  uint `json:"requestId"`
	Version    int  `json:"version"`
	AssigneeID uint `json:"assigneeId"`
}
type DashboardView struct {
	PendingTasks       int64  `json:"pendingTasks"`
	MyRequests         int64  `json:"myRequests"`
	ContractReminders  int64  `json:"contractReminders"`
	WorkflowExceptions *int64 `json:"workflowExceptions"`
	ApprovedLeaves     *int64 `json:"approvedLeaves"`
}
