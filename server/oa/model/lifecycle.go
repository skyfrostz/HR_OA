package model

import (
	"encoding/json"
	"time"
)

const (
	EmploymentActive     = "ACTIVE"
	EmploymentOffboarded = "OFFBOARDED"

	AuditSalary      = "SALARY"
	AuditDepartment  = "DEPARTMENT"
	AuditAnnualLeave = "ANNUAL_LEAVE"
	AuditContract    = "CONTRACT"
	AuditAccount     = "ACCOUNT"
	AuditEmployment  = "EMPLOYMENT"
)

type AuditEvent struct {
	ID           uint            `gorm:"primaryKey" json:"id"`
	Type         string          `gorm:"size:16;not null;index" json:"type"`
	Action       string          `gorm:"size:20;not null" json:"action"`
	ActorID      uint            `gorm:"not null" json:"actorId"`
	EmployeeID   *uint           `gorm:"index" json:"employeeId"`
	DepartmentID *uint           `gorm:"index" json:"departmentId"`
	ContractID   *uint           `gorm:"index" json:"contractId"`
	Reason       string          `gorm:"size:500;not null" json:"reason"`
	Before       json.RawMessage `gorm:"type:json" json:"before" swaggertype:"object"`
	After        json.RawMessage `gorm:"type:json" json:"after" swaggertype:"object"`
	CreatedAt    time.Time       `json:"createdAt"`
}

func (AuditEvent) TableName() string { return "oa_audit_events" }

type AuditFilter struct {
	Type                                 string
	EmployeeID, DepartmentID, ContractID uint
}

type AccountSession struct {
	UserID uint   `gorm:"primaryKey;autoIncrement:false"`
	Epoch  uint64 `gorm:"not null"`
}

func (AccountSession) TableName() string { return "oa_account_sessions" }

// WorkflowGuard serializes lifecycle changes with approval assignments.
type WorkflowGuard struct {
	ID uint `gorm:"primaryKey;autoIncrement:false"`
}

func (WorkflowGuard) TableName() string { return "oa_workflow_guard" }

type ApprovalHandoff struct {
	UserID       uint `gorm:"primaryKey;autoIncrement:false"`
	TargetUserID uint `gorm:"not null"`
}

func (ApprovalHandoff) TableName() string { return "oa_approval_handoffs" }

type AccountDetails struct {
	ID       uint   `json:"id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	Enabled  bool   `json:"enabled"`
}

type EmployeeAccount struct {
	EmployeeID       uint            `json:"employeeId"`
	EmploymentStatus string          `json:"employmentStatus"`
	Version          int             `json:"version"`
	Account          *AccountDetails `json:"account"`
}

type AccountAction struct {
	Action        string  `json:"action"`
	Version       int     `json:"version"`
	Reason        string  `json:"reason"`
	Username      *string `json:"username"`
	Password      *string `json:"password"`
	Role          *string `json:"role"`
	UserID        *uint   `json:"userId"`
	HandoffUserID *uint   `json:"handoffUserId"`
}

type ContractEdit struct {
	Version    int    `json:"version"`
	Reason     string `json:"reason"`
	ContractNo string `json:"contractNo"`
	StartDate  string `json:"startDate"`
	EndDate    string `json:"endDate"`
}

type ContractAction struct {
	Action        string `json:"action"`
	Version       int    `json:"version"`
	Reason        string `json:"reason"`
	EffectiveDate string `json:"effectiveDate"`
}

type DepartmentNode struct {
	DepartmentView
	EmployeeCount int64 `json:"employeeCount"`
}

type DepartmentTree struct {
	List []DepartmentNode `json:"list"`
}

type WorkflowFilter struct {
	Type, Status, FromDate, ToDate string
}
