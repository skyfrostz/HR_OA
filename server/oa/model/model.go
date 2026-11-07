package model

import (
	"time"

	"github.com/flipped-aurora/gin-vue-admin/server/global"
)

const (
	HRAdmin      uint = 9001
	HRViewer     uint = 9002
	EmployeeRole uint = 9003
	ApproverRole uint = 9004
)

func RoleName(authorityID uint) string {
	switch authorityID {
	case HRAdmin:
		return "HR_ADMIN"
	case HRViewer:
		return "HR_VIEWER"
	case EmployeeRole:
		return "EMPLOYEE"
	case ApproverRole:
		return "APPROVER"
	default:
		return ""
	}
}

type Employee struct {
	global.GVA_MODEL
	EmploymentStatus string    `gorm:"size:16;not null;default:ACTIVE"`
	UserID           *uint     `gorm:"uniqueIndex"`
	EmployeeNo       string    `gorm:"size:64;not null;uniqueIndex"`
	Name             string    `gorm:"size:128;not null"`
	BirthDate        time.Time `gorm:"type:date;not null"`
	HireDate         time.Time `gorm:"type:date;not null"`
	Department       string    `gorm:"size:128;not null"`
	DepartmentID     *uint     `gorm:"index"`
	Version          int       `gorm:"not null;default:1"`
	SalaryCents      int64     `gorm:"not null"`
}

func (Employee) TableName() string { return "oa_employees" }

type Contract struct {
	global.GVA_MODEL
	EmployeeID         uint       `gorm:"not null;index"`
	Employee           Employee   `gorm:"foreignKey:EmployeeID"`
	ContractNo         string     `gorm:"size:64;not null;uniqueIndex"`
	StartDate          time.Time  `gorm:"type:date;not null"`
	EndDate            time.Time  `gorm:"type:date;not null"`
	Status             string     `gorm:"size:16;not null"`
	Version            int        `gorm:"not null;default:1"`
	SignedDate         *time.Time `gorm:"type:date"`
	TerminatedDate     *time.Time `gorm:"type:date"`
	PreviousContractID *uint
	PreviousContract   *Contract      `gorm:"foreignKey:PreviousContractID"`
	Files              []ContractFile `gorm:"foreignKey:ContractID"`
}

func (Contract) TableName() string { return "oa_contracts" }

type ContractFile struct {
	global.GVA_MODEL
	ContractID   uint   `gorm:"not null;index"`
	OriginalName string `gorm:"size:255;not null"`
	Size         int64  `gorm:"not null"`
	ContentType  string `gorm:"size:32;not null"`
	StorageKey   string `gorm:"size:32;not null;uniqueIndex"`
}

func (ContractFile) TableName() string { return "oa_contract_files" }
