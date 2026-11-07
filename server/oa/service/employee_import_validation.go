package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/flipped-aurora/gin-vue-admin/server/oa/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type employeeRowLookup struct {
	Row        int    `json:"row"`
	EmployeeNo string `json:"employeeNo"`
	Department string `json:"department"`
}

type employeeRowMatch struct {
	Row            int `gorm:"column:worksheet_row"`
	DuplicateCount int
	ExistingID     *uint
	DepartmentID   *uint
	DepartmentName *string
}

func stageEmployeeRows(db *gorm.DB, rows []model.ImportRow, now time.Time) ([]stagedEmployee, error) {
	lookups := make([]employeeRowLookup, 0, len(rows))
	staged := make([]stagedEmployee, 0, len(rows))
	inputs := make([]model.EmployeeInput, len(rows))
	for i := range rows {
		row := &rows[i]
		inputs[i] = validateEmployeeRow(row, now)
		lookup := employeeRowLookup{Row: row.Row}
		if validText(row.Values.EmployeeNo, 64) {
			lookup.EmployeeNo = row.Values.EmployeeNo
		}
		if validText(row.Values.Department, 128) {
			lookup.Department = row.Values.Department
		}
		lookups = append(lookups, lookup)
	}
	matches, err := matchEmployeeRows(db, lookups)
	if err != nil {
		return nil, err
	}
	for i := range rows {
		row, lookup := &rows[i], lookups[i]
		match := matches[row.Row]
		if lookup.EmployeeNo != "" {
			if match.DuplicateCount > 1 {
				rowError(row, "employeeNo", "工作簿中工号重复")
			}
			if match.ExistingID != nil {
				rowError(row, "employeeNo", "工号已存在")
			}
		}
		if lookup.Department != "" && match.DepartmentID == nil {
			rowError(row, "department", "部门不存在")
		}
		if len(row.Errors) == 0 && match.DepartmentID != nil && match.DepartmentName != nil {
			input := inputs[i]
			input.Department, input.DepartmentID = *match.DepartmentName, match.DepartmentID
			staged = append(staged, stagedEmployee{Row: row.Row, Input: input})
		}
	}
	return staged, nil
}

func validateEmployeeRow(row *model.ImportRow, now time.Time) model.EmployeeInput {
	values := row.Values
	for _, field := range []struct {
		name, value, message string
		limit                int
	}{
		{"employeeNo", values.EmployeeNo, "工号须为 1–64 字符", 64},
		{"name", values.Name, "姓名须为 1–128 字符", 128},
		{"department", values.Department, "部门须为 1–128 字符", 128},
	} {
		if !validText(field.value, field.limit) {
			rowError(row, field.name, field.message)
		}
	}
	birth, birthErr := parseDate(values.BirthDate)
	hire, hireErr := parseDate(values.HireDate)
	if birthErr != nil || values.BirthDate > now.Format(dateLayout) {
		rowError(row, "birthDate", "出生日期须为 YYYY-MM-DD，且不能晚于今天")
	}
	if hireErr != nil || (birthErr == nil && hire.Before(birth)) {
		rowError(row, "hireDate", "入职日期须为 YYYY-MM-DD，且不能早于出生日期")
	}
	salary, salaryErr := parseSalaryYuan(values.Salary)
	if salaryErr != nil {
		rowError(row, "salary", "工资须为非负金额，最多两位小数且不超过安全分值上限")
	}
	return model.EmployeeInput{
		EmployeeNo: values.EmployeeNo, Name: values.Name, Department: values.Department,
		BirthDate: values.BirthDate, HireDate: values.HireDate, SalaryCents: &salary,
	}
}

func rowError(row *model.ImportRow, field, message string) {
	row.Errors = append(row.Errors, model.ImportRowError{Field: field, Message: message})
}

func parseSalaryYuan(value string) (int64, error) {
	whole, fraction, decimal := strings.Cut(value, ".")
	if whole == "" || (decimal && (len(fraction) == 0 || len(fraction) > 2)) {
		return 0, ErrInvalid
	}
	for _, part := range []string{whole, fraction} {
		for _, digit := range part {
			if digit < '0' || digit > '9' {
				return 0, ErrInvalid
			}
		}
	}
	whole = strings.TrimLeft(whole, "0")
	// Convert decimal digits directly so binary floating point is never used.
	cents, err := strconv.ParseInt(whole+fraction+strings.Repeat("0", 2-len(fraction)), 10, 64)
	if err != nil || cents > maxSalaryCents {
		return 0, ErrInvalid
	}
	return cents, nil
}

func confirmableEmployees(tx *gorm.DB, staged []stagedEmployee, now time.Time) ([]model.Employee, error) {
	departmentIDs := make([]uint, 0, len(staged))
	lookups := make([]employeeRowLookup, 0, len(staged))
	for _, entry := range staged {
		if entry.Input.DepartmentID == nil {
			return nil, ErrImportConflict
		}
		departmentIDs = append(departmentIDs, *entry.Input.DepartmentID)
		lookups = append(lookups, employeeRowLookup{
			Row: entry.Row, EmployeeNo: entry.Input.EmployeeNo, Department: entry.Input.Department,
		})
	}
	var departments []model.Department
	if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).Where("id IN ?", departmentIDs).Order("id ASC").Find(&departments).Error; err != nil {
		return nil, err
	}
	matches, err := matchEmployeeRows(tx, lookups)
	if err != nil {
		return nil, err
	}
	employees := make([]model.Employee, 0, len(staged))
	for _, entry := range staged {
		match := matches[entry.Row]
		if match.DuplicateCount != 1 || match.ExistingID != nil || match.DepartmentID == nil || match.DepartmentName == nil ||
			*match.DepartmentID != *entry.Input.DepartmentID || *match.DepartmentName != entry.Input.Department {
			return nil, ErrImportConflict
		}
		employee, err := employeeFromInput(entry.Input, now)
		if err != nil {
			return nil, ErrImportConflict
		}
		employees = append(employees, employee)
	}
	return employees, nil
}

func matchEmployeeRows(db *gorm.DB, lookups []employeeRowLookup) (map[int]employeeRowMatch, error) {
	result := make(map[int]employeeRowMatch, len(lookups))
	if len(lookups) == 0 {
		return result, nil
	}
	employeeCollation, err := workbookCollation(db, "oa_employees", "employee_no")
	if err != nil {
		return nil, err
	}
	departmentCollation, err := workbookCollation(db, "oa_departments", "name")
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(lookups)
	if err != nil {
		return nil, err
	}
	// Match duplicates using the same collations as the actual unique indexes.
	// Collation identifiers come from the schema and pass a strict allowlist.
	query := fmt.Sprintf(`
		SELECT x.worksheet_row,
			COUNT(*) OVER (PARTITION BY x.employee_no COLLATE %s) AS duplicate_count,
			e.id AS existing_id, d.id AS department_id, d.name AS department_name
		FROM JSON_TABLE(?, '$[*]' COLUMNS (
			worksheet_row INT PATH '$.row',
			employee_no VARCHAR(64) CHARACTER SET utf8mb4 PATH '$.employeeNo',
			department VARCHAR(128) CHARACTER SET utf8mb4 PATH '$.department'
		)) AS x
		LEFT JOIN oa_employees AS e ON e.employee_no = x.employee_no COLLATE %s
		LEFT JOIN oa_departments AS d ON d.name = x.department COLLATE %s`, employeeCollation, employeeCollation, departmentCollation)
	var matches []employeeRowMatch
	if err := db.Raw(query, string(payload)).Scan(&matches).Error; err != nil {
		return nil, err
	}
	for _, match := range matches {
		result[match.Row] = match
	}
	if len(result) != len(lookups) {
		return nil, errors.New("employee import lookup incomplete")
	}
	return result, nil
}

func workbookCollation(db *gorm.DB, table, column string) (string, error) {
	var name string
	err := db.Raw(`SELECT COLLATION_NAME FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND COLUMN_NAME = ?`, table, column).Scan(&name).Error
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(name, "utf8mb4_") || len(name) > 64 {
		return "", errors.New("unsupported employee import collation")
	}
	for _, c := range name {
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '_') {
			return "", errors.New("invalid employee import collation")
		}
	}
	return name, nil
}
