package model

import (
	"encoding/json"
	"time"
)

type Department struct {
	ID       uint   `gorm:"primaryKey"`
	Name     string `gorm:"size:128;not null;uniqueIndex"`
	ParentID *uint  `gorm:"index"`
	Version  int    `gorm:"not null;default:1"`
}

func (Department) TableName() string { return "oa_departments" }

type AnnualLeave struct {
	ID               uint      `gorm:"primaryKey"`
	EmployeeID       uint      `gorm:"not null;uniqueIndex:ux_oa_employee_leave,priority:1"`
	Year             int       `gorm:"not null;uniqueIndex:ux_oa_employee_leave,priority:2"`
	EntitledHalfDays int       `gorm:"not null"`
	UsedHalfDays     int       `gorm:"not null"`
	Note             string    `gorm:"type:text;not null"`
	Version          int       `gorm:"not null;default:1"`
	UpdatedAt        time.Time `gorm:"not null"`
}

func (AnnualLeave) TableName() string { return "oa_annual_leave" }

const (
	ImportPending   = "PREVIEW"
	ImportCommitted = "COMMITTED"
)

type EmployeeImport struct {
	ConfirmedAt *time.Time
	PurgedAt    *time.Time      `gorm:"index:ix_oa_import_cleanup,priority:1"`
	ID          string          `gorm:"type:char(32);primaryKey"`
	CreatorID   uint            `gorm:"not null;index"`
	Status      string          `gorm:"size:16;not null"`
	CreatedAt   time.Time       `gorm:"not null"`
	ExpiresAt   time.Time       `gorm:"not null;index:ix_oa_import_cleanup,priority:2"`
	Preview     json.RawMessage `gorm:"type:json;not null"`
	Validated   json.RawMessage `gorm:"type:json"`
	Result      json.RawMessage `gorm:"type:json"`
}

func (EmployeeImport) TableName() string { return "oa_employee_imports" }
