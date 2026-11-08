package service

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/flipped-aurora/gin-vue-admin/server/oa/model"
	"github.com/xuri/excelize/v2"
)

func TestEmployeeImportDistinguishesDateErrors(t *testing.T) {
	now := time.Date(2026, 11, 8, 0, 1, 0, 0, businessTime)
	for _, tc := range []struct {
		name, birth, hire, field, code string
	}{
		{"format", "1990-1-01", "2020-01-01", "birthDate", "DATE_FORMAT"},
		{"non-ASCII digits", "１９９０-０１-０１", "2020-01-01", "birthDate", "DATE_FORMAT"},
		{"nonexistent birth date", "1990-02-30", "2020-01-01", "birthDate", "DATE_NOT_FOUND"},
		{"nonexistent hire date", "1990-01-01", "2025-02-29", "hireDate", "DATE_NOT_FOUND"},
		{"unsupported year", "0999-12-31", "2020-01-01", "birthDate", "DATE_OUT_OF_RANGE"},
		{"future birth date", "2026-11-09", "2026-11-10", "birthDate", "FUTURE_BIRTH_DATE"},
		{"hire before birth", "1990-01-02", "1990-01-01", "hireDate", "HIRE_BEFORE_BIRTH"},
		{"leap date and future hire", "2000-02-29", "2026-11-09", "", ""},
		{"local today", "2026-11-08", "2026-11-08", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			row := model.ImportRow{Values: model.ImportValues{
				EmployeeNo: "TEST-1", Name: "演示人员", Department: "演示部门",
				BirthDate: tc.birth, HireDate: tc.hire, Salary: "0.00",
			}}
			validateEmployeeRow(&row, now)
			if tc.code == "" {
				if len(row.Errors) != 0 {
					t.Fatalf("valid date rejected: %+v", row.Errors)
				}
			} else if len(row.Errors) != 1 || row.Errors[0].Field != tc.field ||
				row.Errors[0].Code != tc.code || row.Errors[0].Message == "" {
				t.Fatalf("expected %s/%s, got %+v", tc.field, tc.code, row.Errors)
			}
		})
	}
}

func TestEmployeeImportTemplateAndPhysicalRows(t *testing.T) {
	service := new(Service)
	data, err := service.EmployeeImportTemplate(model.Actor{AuthorityID: model.HRAdmin})
	if err != nil {
		t.Fatal(err)
	}
	if rows, err := readEmployeeWorkbook(bytes.NewReader(data)); err != nil || len(rows) != 0 {
		t.Fatalf("blank template was rejected or contained sample rows: %d, %v", len(rows), err)
	}
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	styleID, err := f.GetColStyle(importSheetName, "A")
	if err != nil || styleID == 0 {
		t.Fatalf("input column lacks text style: %d, %v", styleID, err)
	}
	for _, row := range []int{3, 8} {
		if err := writeExcelTextRow(f, importSheetName, row, []string{"001", "演示", "现有部门", "1990-01-01", "2020-01-01", "5.30"}); err != nil {
			t.Fatal(err)
		}
	}
	data, err = workbookBytes(f)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := readEmployeeWorkbook(bytes.NewReader(data))
	if err != nil || len(rows) != 2 || rows[0].Row != 3 || rows[1].Row != 8 || rows[0].Values.EmployeeNo != "001" {
		t.Fatalf("unexpected workbook rows: %+v, %v", rows, err)
	}
}

func TestEmployeeWorkbookRejectsFormulaExternalLinkAndInvalidDimensions(t *testing.T) {
	for _, test := range []struct {
		name string
		edit func(*excelize.File) error
		want error
	}{
		{"formula with saved value", func(f *excelize.File) error {
			if err := f.SetCellInt(importSheetName, "A2", 10); err != nil {
				return err
			}
			return f.SetCellFormula(importSheetName, "A2", "=2+8")
		}, ErrInvalid},
		{"external hyperlink", func(f *excelize.File) error {
			return f.SetCellHyperLink(importSheetName, "A2", "https://example.invalid", "External")
		}, ErrInvalid},
		{"extra column", func(f *excelize.File) error {
			return f.SetCellStr(importSheetName, "G2", "invalid")
		}, ErrInvalid},
		{"oversized row", func(f *excelize.File) error {
			return f.SetCellStr(importSheetName, "A10002", "invalid")
		}, ErrTooLarge},
		{"additional sheet", func(f *excelize.File) error {
			_, err := f.NewSheet("第二表")
			return err
		}, ErrInvalid},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := excelize.NewFile()
			defer f.Close()
			if err := f.SetSheetName("Sheet1", importSheetName); err != nil {
				t.Fatal(err)
			}
			if err := writeExcelTextRow(f, importSheetName, 1, importHeaders); err != nil {
				t.Fatal(err)
			}
			if err := test.edit(f); err != nil {
				t.Fatal(err)
			}
			data, err := workbookBytes(f)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := readEmployeeWorkbook(bytes.NewReader(data)); !errors.Is(err, test.want) {
				t.Fatalf("got %v, want %v", err, test.want)
			}
		})
	}
}

func TestEmployeeExportHasLiteralCellsAndSalaryOnlyWhenAllowed(t *testing.T) {
	const text = "=1+1"
	rows := []model.Employee{
		{EmployeeNo: "0007", Name: text, Department: "+1", SalaryCents: maxSalaryCents},
	}
	for _, includeSalary := range []bool{false, true} {
		data, err := employeeExportWorkbook(rows, includeSalary, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		f, err := excelize.OpenReader(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		sheet := f.GetSheetName(0)
		content, err := f.GetRows(sheet, excelize.Options{RawCellValue: true})
		if err != nil {
			t.Fatal(err)
		}
		if len(content) != 2 || content[1][0] != "0007" || content[1][1] != text || content[1][2] != "+1" {
			t.Fatalf("formula-looking user text changed: %v", content)
		}
		formula, err := f.GetCellFormula(sheet, "B2")
		if err != nil || formula != "" {
			t.Fatalf("text became an executable formula: %q, %v", formula, err)
		}
		if includeSalary {
			if len(content[0]) != 8 || content[0][7] != "约定工资（元）" || content[1][7] != "90071992547409.91" {
				t.Fatalf("salary is not exact: %v", content)
			}
		} else if len(content[0]) != 7 || len(content[1]) != 7 {
			t.Fatalf("unauthorized salary column found: %v", content)
		}
	}
}

func TestSalaryYuanExactParsing(t *testing.T) {
	for value, want := range map[string]int64{"0": 0, "0.01": 1, "05.3": 530, "90071992547409.91": maxSalaryCents} {
		got, err := parseSalaryYuan(value)
		if err != nil || got != want {
			t.Errorf("%q got %d, %v, want %d", value, got, err, want)
		}
	}
	for _, value := range []string{"", "-1", "1e2", "1.003", "1.", ".3", "90071992547409.92", strings.Repeat("9", 64)} {
		if _, err := parseSalaryYuan(value); !errors.Is(err, ErrInvalid) {
			t.Errorf("%q should be invalid, got %v", value, err)
		}
	}
}

func TestWorkbookWorksheetRejectsContradictoryCoordinates(t *testing.T) {
	for _, xml := range []string{
		`<worksheet><sheetData><row r="2"><c r="A1"/></row></sheetData></worksheet>`,
		`<worksheet><sheetData><row r="3"/><row r="2"/></sheetData></worksheet>`,
	} {
		if _, err := auditWorkbookXML(strings.NewReader(xml)); !errors.Is(err, ErrInvalid) {
			t.Fatalf("invalid row coordinates were accepted: %v", err)
		}
	}
}
