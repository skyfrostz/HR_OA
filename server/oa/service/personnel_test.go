package service

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/flipped-aurora/gin-vue-admin/server/oa/model"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestDepartmentTreeRejectsMissingParentsCyclesAndDepth(t *testing.T) {
	pointer := func(id uint) *uint { return &id }
	parents := map[uint]*uint{1: nil, 2: pointer(1), 3: pointer(2)}
	if !validDepartmentTree(parents) {
		t.Fatal("valid tree rejected")
	}
	for _, invalid := range []uint{1, 3, 900} {
		parents[1] = pointer(invalid)
		if validDepartmentTree(parents) {
			t.Fatalf("unexpected root parent accepted: %d", invalid)
		}
	}
	parents[1] = nil
	for id := uint(4); id <= 20; id++ {
		parents[id] = pointer(id - 1)
	}
	if !validDepartmentTree(parents) {
		t.Fatal("depth 20 rejected")
	}
	parents[21] = pointer(20)
	if validDepartmentTree(parents) {
		t.Fatal("depth 21 accepted")
	}
}

func TestEmployeeInputSupportsLegacyAndDepartmentID(t *testing.T) {
	now := time.Date(2026, 11, 7, 0, 1, 0, 0, businessTime)
	department, missing := uint(8), uint(0)
	salary := int64(0)
	input := model.EmployeeInput{
		EmployeeNo: "  TEST  ", Name: "演示人员", BirthDate: "1990-01-01", HireDate: "2020-01-01",
		Department: " 演示部门 ", SalaryCents: &salary,
	}
	employee, err := employeeFromInput(input, now)
	if err != nil || employee.Department != "演示部门" || employee.EmployeeNo != "TEST" || employee.Version != 1 {
		t.Fatalf("legacy create changed: %+v, %v", employee, err)
	}
	input.Department, input.DepartmentID = "", &department
	employee, err = employeeFromInput(input, now)
	if err != nil || employee.DepartmentID == nil || *employee.DepartmentID != department {
		t.Fatalf("department ID was not accepted: %+v, %v", employee, err)
	}
	input.DepartmentID = &missing
	if _, err := employeeFromInput(input, now); !errors.Is(err, ErrInvalid) {
		t.Fatalf("zero department ID was not rejected: %v", err)
	}
}

func TestAnnualLeaveUsesExactHalfDaysAndVersion(t *testing.T) {
	entitled, used, version := 11, 4, 0
	input := model.AnnualLeaveInput{
		EntitledHalfDays: &entitled, UsedHalfDays: &used, Version: &version, Note: "  手动登记  ",
	}
	row, err := annualLeaveFromInput(1, 2026, input)
	if err != nil || row.Version != 1 || row.Note != "手动登记" || annualLeaveView(row).RemainingHalfDays != 7 {
		t.Fatalf("unexpected annual leave row: %+v, %v", row, err)
	}
	version = 2
	row, err = annualLeaveFromInput(1, 2026, input)
	if err != nil || row.Version != 3 {
		t.Fatalf("edit did not derive next version: %+v, %v", row, err)
	}
	used = 12
	if _, err := annualLeaveFromInput(1, 2026, input); !errors.Is(err, ErrInvalid) {
		t.Fatalf("used > entitled was not rejected: %v", err)
	}
	input.Version = nil
	used = 0
	if _, err := annualLeaveFromInput(1, 2026, input); !errors.Is(err, ErrInvalid) {
		t.Fatalf("missing version was not rejected: %v", err)
	}
}

func TestExpiryCalendarDayDifference(t *testing.T) {
	now := time.Date(2026, 11, 7, 0, 10, 0, 0, businessTime)
	for _, test := range []struct {
		end  string
		days int64
	}{
		{"2026-11-06", -1},
		{"2026-11-07", 0},
		{"2026-12-07", 30},
		{"1600-11-07", -155593},
	} {
		end, err := parseDate(test.end)
		if err != nil {
			t.Fatal(err)
		}
		if got := calendarDaysBetween(now, end); got != test.days {
			t.Errorf("end %s got %d days, want %d", test.end, got, test.days)
		}
	}
}

func TestExportQueryUsesEmployeeScopeAndSalaryProjection(t *testing.T) {
	pool, err := sql.Open("mysql", "")
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	db, err := gorm.Open(mysql.New(mysql.Config{Conn: pool, SkipInitializeWithVersion: true}), &gorm.Config{
		DryRun: true, DisableAutomaticPing: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	s := New(db, "", nil, nil)
	id := uint(12)
	for _, role := range []uint{model.EmployeeRole, model.ApproverRole} {
		stmt := s.employeeProjection(context.Background(), model.Actor{AuthorityID: role, UserID: 23, EmployeeID: &id}).
			Find(&[]model.Employee{}).Statement
		if !strings.Contains(stmt.SQL.String(), "oa_employees.id = ? AND oa_employees.user_id = ?") ||
			!reflect.DeepEqual(stmt.Vars, []any{uint(12), uint(23)}) {
			t.Fatalf("role %d missing row scope: %s, %v", role, stmt.SQL.String(), stmt.Vars)
		}
	}
	viewer := s.employeeProjection(context.Background(), model.Actor{AuthorityID: model.HRViewer}).
		Find(&[]model.Employee{}).Statement.SQL.String()
	if strings.Contains(viewer, "salary_cents") || strings.Contains(viewer, "SELECT *") {
		t.Fatalf("viewer export selected salary: %s", viewer)
	}
}
