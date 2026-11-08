package service

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/flipped-aurora/gin-vue-admin/server/oa/model"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestEmployeeSearchInputBounds(t *testing.T) {
	ptr := func(n int) *int { return &n }
	for _, test := range []struct {
		input model.EmployeeSearch
		valid bool
	}{
		{model.EmployeeSearch{}, true},
		{model.EmployeeSearch{Page: ptr(1), PageSize: ptr(100)}, true},
		{model.EmployeeSearch{Page: ptr(0)}, false},
		{model.EmployeeSearch{Page: ptr(-1)}, false},
		{model.EmployeeSearch{PageSize: ptr(0)}, false},
		{model.EmployeeSearch{PageSize: ptr(101)}, false},
		{model.EmployeeSearch{Page: ptr(math.MaxInt), PageSize: ptr(100)}, false},
	} {
		page, err := employeeSearchPage(test.input)
		if (err == nil) != test.valid {
			t.Fatalf("unexpected page validation: %+v, %v", page, err)
		}
	}
	page, _ := employeeSearchPage(model.EmployeeSearch{})
	if page.Page != 1 || page.PageSize != 20 {
		t.Fatal("default pagination changed")
	}
	for _, query := range []string{strings.Repeat("名", 101), "a\x00b", "a\nb", "a\rb", string([]byte{0xff})} {
		if _, err := normalizeEmployeeFilter(model.EmployeeFilter{Query: query}); !errors.Is(err, ErrInvalid) {
			t.Fatal("invalid search accepted")
		}
	}
	zero := uint(0)
	if _, err := normalizeEmployeeFilter(model.EmployeeFilter{DepartmentID: &zero}); !errors.Is(err, ErrInvalid) {
		t.Fatal("zero department accepted")
	}
	filter, err := normalizeEmployeeFilter(model.EmployeeFilter{Query: "  " + strings.Repeat("名", 100) + "  "})
	if err != nil || filter.Query != strings.Repeat("名", 100) {
		t.Fatal("valid Unicode boundary or whitespace handling failed")
	}
}

func TestEmployeeSearchValidatesDepartmentScope(t *testing.T) {
	department := uint(8)
	for _, tc := range []struct {
		scope string
		id    *uint
		valid bool
	}{
		{"", nil, true},
		{"DIRECT", nil, true},
		{"DIRECT", &department, true},
		{"SUBTREE", &department, true},
		{"SUBTREE", nil, false},
		{"subtree", &department, false},
		{"UNKNOWN", &department, false},
	} {
		_, err := normalizeEmployeeFilter(model.EmployeeFilter{DepartmentID: tc.id, DepartmentScope: tc.scope})
		if tc.valid && err != nil || !tc.valid && !errors.Is(err, ErrInvalid) {
			t.Errorf("scope %q, id %v: got %v", tc.scope, tc.id, err)
		}
	}
}

func TestEmployeeSearchCannotWidenRowScopeOrSelectViewerSalary(t *testing.T) {
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
	id, department := uint(12), uint(8)
	query := `%_' OR 1=1 -- \`
	filter := model.EmployeeFilter{Query: query, DepartmentID: &department}
	for _, role := range []uint{model.EmployeeRole, model.ApproverRole} {
		actor := model.Actor{AuthorityID: role, UserID: 23, EmployeeID: &id}
		stmt := filterEmployees(s.employeeProjection(context.Background(), actor), filter).
			Find(&[]model.Employee{}).Statement
		sqlText := stmt.SQL.String()
		if !strings.Contains(sqlText, "oa_employees.id = ? AND oa_employees.user_id = ?") ||
			!strings.Contains(sqlText, "AND ((LOCATE(") || strings.Contains(sqlText, query) ||
			!reflect.DeepEqual(stmt.Vars, []any{id, uint(23), query, query, query, department}) {
			t.Fatalf("search weakened scope or interpolated text: %s / %v", sqlText, stmt.Vars)
		}
	}
	viewerSQL := filterEmployees(s.employeeProjection(context.Background(), model.Actor{AuthorityID: model.HRViewer}), filter).
		Find(&[]model.Employee{}).Statement.SQL.String()
	if strings.Contains(viewerSQL, "salary_cents") || strings.Contains(viewerSQL, "SELECT *") {
		t.Fatalf("viewer search selected salary: %s", viewerSQL)
	}
	deniedSQL := filterEmployees(s.visibleEmployees(context.Background(), model.Actor{AuthorityID: model.EmployeeRole}), filter).
		Find(&[]model.Employee{}).Statement.SQL.String()
	if !strings.Contains(deniedSQL, "1 = 0 AND ((LOCATE(") {
		t.Fatalf("unbound identity gained search scope: %s", deniedSQL)
	}
}
