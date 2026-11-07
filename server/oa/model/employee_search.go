package model

// EmployeeFilter stays in a JSON body so personal search text never enters URLs.
type EmployeeFilter struct {
	Query        string `json:"query"`
	DepartmentID *uint  `json:"departmentId"`
}

type EmployeeSearch struct {
	EmployeeFilter
	Page     *int `json:"page"`
	PageSize *int `json:"pageSize"`
}
