package service

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/flipped-aurora/gin-vue-admin/server/oa/model"
)

func TestEmployeeInputSalaryBounds(t *testing.T) {
	now := time.Date(2026, 11, 6, 12, 0, 0, 0, businessTime)
	zero, negative, maxSalary, tooHigh := int64(0), int64(-1), int64(9007199254740991), int64(9007199254740992)
	for _, test := range []struct {
		name   string
		salary *int64
		valid  bool
	}{
		{"missing", nil, false},
		{"negative", &negative, false},
		{"zero", &zero, true},
		{"maximum", &maxSalary, true},
		{"over maximum", &tooHigh, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := model.EmployeeInput{
				EmployeeNo: "TEST", Name: "演示", Department: "演示部门",
				BirthDate: "1990-01-01", HireDate: "2020-01-01", SalaryCents: test.salary,
			}
			got, err := employeeFromInput(input, now)
			if test.valid {
				if err != nil || got.SalaryCents != *test.salary {
					t.Fatalf("expected exact salary %d, got %d, error %v", *test.salary, got.SalaryCents, err)
				}
			} else if !errors.Is(err, ErrInvalid) {
				t.Fatalf("expected invalid salary, got %v", err)
			}
		})
	}
}

func TestEmployeeInputUsesShanghaiCalendarDate(t *testing.T) {
	now := time.Date(2026, 11, 6, 0, 5, 0, 0, businessTime)
	salary := int64(0)
	input := model.EmployeeInput{
		EmployeeNo: "TEST", Name: "演示", Department: "演示部门",
		BirthDate: "2026-11-06", HireDate: "2026-11-07", SalaryCents: &salary,
	}
	employee, err := employeeFromInput(input, now)
	if err != nil || employee.BirthDate.Format(dateLayout) != input.BirthDate || employee.BirthDate.Location() != time.UTC {
		t.Fatalf("expected Shanghai today accepted and preserved as a DATE, got %v, error %v", employee.BirthDate, err)
	}
	input.BirthDate = "2026-11-07"
	if _, err := employeeFromInput(input, now); !errors.Is(err, ErrInvalid) {
		t.Fatalf("expected future birth date rejected, got %v", err)
	}
}

func TestSalaryOmittedForViewerAndPreservedForAuthorizedUser(t *testing.T) {
	employee := model.Employee{SalaryCents: 0}
	for _, canRead := range []bool{false, true} {
		raw, err := json.Marshal(employeeView(employee, canRead, time.Now()))
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil {
			t.Fatal(err)
		}
		salary, present := fields["salaryCents"]
		if present != canRead || (present && string(salary) != "0") {
			t.Fatalf("permission %t, salary field present %t, value %s", canRead, present, salary)
		}
	}
}
