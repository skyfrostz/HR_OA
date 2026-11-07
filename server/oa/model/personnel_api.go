package model

import "time"

type DepartmentInput struct {
	Name     string `json:"name"`
	ParentID *uint  `json:"parentId"`
}

type DepartmentEdit struct {
	DepartmentInput
	Version int `json:"version"`
}

type DepartmentView struct {
	ID       uint   `json:"id"`
	Name     string `json:"name"`
	ParentID *uint  `json:"parentId"`
	Version  int    `json:"version"`
}

type EmployeeEdit struct {
	EmployeeInput
	Version int `json:"version"`
}

type AnnualLeaveInput struct {
	EntitledHalfDays *int   `json:"entitledHalfDays"`
	UsedHalfDays     *int   `json:"usedHalfDays"`
	Note             string `json:"note"`
	Version          *int   `json:"version"`
}

type AnnualLeaveView struct {
	ID                uint      `json:"id"`
	EmployeeID        uint      `json:"employeeId"`
	Year              int       `json:"year"`
	EntitledHalfDays  int       `json:"entitledHalfDays"`
	UsedHalfDays      int       `json:"usedHalfDays"`
	RemainingHalfDays int       `json:"remainingHalfDays"`
	Note              string    `json:"note"`
	Version           int       `json:"version"`
	UpdatedAt         time.Time `json:"updatedAt"`
}

type ImportValues struct {
	EmployeeNo string `json:"employeeNo"`
	Name       string `json:"name"`
	Department string `json:"department"`
	BirthDate  string `json:"birthDate"`
	HireDate   string `json:"hireDate"`
	Salary     string `json:"salary"`
}

type ImportRowError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

type ImportRow struct {
	Row    int              `json:"row"`
	Values ImportValues     `json:"values"`
	Errors []ImportRowError `json:"errors"`
}

type ImportPreview struct {
	ID         string      `json:"id"`
	CreatedAt  time.Time   `json:"createdAt"`
	ExpiresAt  time.Time   `json:"expiresAt"`
	Status     string      `json:"status"`
	TotalRows  int         `json:"totalRows"`
	ValidRows  int         `json:"validRows"`
	ErrorRows  int         `json:"errorRows"`
	CanConfirm bool        `json:"canConfirm"`
	Rows       []ImportRow `json:"rows"`
}

type ImportResult struct {
	PreviewID     string `json:"previewId"`
	ImportedCount int    `json:"importedCount"`
	EmployeeIDs   []uint `json:"employeeIds"`
}

type ContractReminder struct {
	ContractID    uint   `json:"contractId"`
	ContractNo    string `json:"contractNo"`
	EmployeeID    uint   `json:"employeeId"`
	EmployeeNo    string `json:"employeeNo"`
	EmployeeName  string `json:"employeeName"`
	EndDate       string `json:"endDate"`
	DaysRemaining int64  `json:"daysRemaining"`
	Status        string `json:"status"`
}
