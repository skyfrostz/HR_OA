package model

import "time"

type Actor struct {
	UserID      uint
	AuthorityID uint
	EmployeeID  *uint
}

func (a Actor) CanWrite() bool { return a.AuthorityID == HRAdmin }
func (a Actor) CanReadSalary() bool {
	return a.AuthorityID == HRAdmin || a.CanSubmitRequests()
}

func (a Actor) CanSubmitRequests() bool {
	return a.EmployeeID != nil && (a.AuthorityID == EmployeeRole || a.AuthorityID == ApproverRole)
}

func (a Actor) CanApproveRequests() bool { return a.AuthorityID == ApproverRole }

type UserView struct {
	ID          uint   `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
	Role        string `json:"role"`
	EmployeeID  *uint  `json:"employeeId"`
}

type Permissions struct {
	CanWriteEmployees    bool `json:"canWriteEmployees"`
	CanReadSalary        bool `json:"canReadSalary"`
	CanSubmitRequests    bool `json:"canSubmitRequests"`
	CanApproveRequests   bool `json:"canApproveRequests"`
	CanManageDepartments bool `json:"canManageDepartments"`
	CanManageAnnualLeave bool `json:"canManageAnnualLeave"`
	CanImportEmployees   bool `json:"canImportEmployees"`
	CanExportEmployees   bool `json:"canExportEmployees"`
}

type SessionView struct {
	User        UserView    `json:"user"`
	Permissions Permissions `json:"permissions"`
}

type LoginInput struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type LoginView struct {
	SessionView
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type EmployeeInput struct {
	EmployeeNo   string `json:"employeeNo"`
	Name         string `json:"name"`
	BirthDate    string `json:"birthDate"`
	HireDate     string `json:"hireDate"`
	Department   string `json:"department"`
	DepartmentID *uint  `json:"departmentId"`
	SalaryCents  *int64 `json:"salaryCents"`
}

type EmployeeView struct {
	ID           uint   `json:"id"`
	EmployeeNo   string `json:"employeeNo"`
	Name         string `json:"name"`
	BirthDate    string `json:"birthDate"`
	HireDate     string `json:"hireDate"`
	Department   string `json:"department"`
	DepartmentID *uint  `json:"departmentId"`
	Version      int    `json:"version"`
	Age          int    `json:"age"`
	TenureYears  int    `json:"tenureYears"`
	SalaryCents  *int64 `json:"salaryCents,omitempty"`
}

type ContractInput struct {
	ContractNo         string `json:"contractNo"`
	StartDate          string `json:"startDate"`
	EndDate            string `json:"endDate"`
	Status             string `json:"status"`
	PreviousContractID *uint  `json:"previousContractId"`
}

type ContractView struct {
	ID                 uint       `json:"id"`
	EmployeeID         uint       `json:"employeeId"`
	ContractNo         string     `json:"contractNo"`
	StartDate          string     `json:"startDate"`
	EndDate            string     `json:"endDate"`
	Status             string     `json:"status"`
	PreviousContractID *uint      `json:"previousContractId"`
	Files              []FileView `json:"files"`
}

type FileView struct {
	ID           uint      `json:"id"`
	OriginalName string    `json:"originalName"`
	Size         int64     `json:"size"`
	ContentType  string    `json:"contentType"`
	CreatedAt    time.Time `json:"createdAt"`
}

type Pagination struct {
	Page     int
	PageSize int
}

func (p Pagination) Offset() int { return (p.Page - 1) * p.PageSize }

type Page[T any] struct {
	List     []T   `json:"list"`
	Total    int64 `json:"total"`
	Page     int   `json:"page"`
	PageSize int   `json:"pageSize"`
}

func NewPage[T any](p Pagination) Page[T] {
	return Page[T]{List: []T{}, Page: p.Page, PageSize: p.PageSize}
}
