package service

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/flipped-aurora/gin-vue-admin/server/oa/model"
	"github.com/xuri/excelize/v2"
)

const (
	MaxEmployeeWorkbookSize = 2 * 1024 * 1024
	maxWorkbookExpansion    = 16 * 1024 * 1024
	maxWorkbookEntries      = 256
	maxImportRows           = 1000
	maxWorksheetRow         = 10001
	maxImportCellBytes      = 4096
	maxExportRows           = 5000
	importSheetName         = "员工导入"
)

var (
	importHeaders  = []string{"工号", "姓名", "部门", "出生日期", "入职日期", "约定工资（元）"}
	ErrExportLimit = errors.New("employee export row limit")
)

func (s *Service) EmployeeImportTemplate(actor model.Actor) ([]byte, error) {
	if !actor.CanWrite() {
		return nil, ErrForbidden
	}
	f := excelize.NewFile()
	defer f.Close()
	if err := f.SetSheetName("Sheet1", importSheetName); err != nil {
		return nil, err
	}
	style, err := f.NewStyle(&excelize.Style{NumFmt: 49})
	if err != nil {
		return nil, err
	}
	if err := f.SetColStyle(importSheetName, "A:F", style); err != nil {
		return nil, err
	}
	if err := writeExcelTextRow(f, importSheetName, 1, importHeaders); err != nil {
		return nil, err
	}
	return workbookBytes(f)
}

func (s *Service) ExportEmployees(ctx context.Context, actor model.Actor) ([]byte, error) {
	if model.RoleName(actor.AuthorityID) == "" {
		return nil, ErrForbidden
	}
	var rows []model.Employee
	err := s.employeeProjection(ctx, actor).Order("id ASC").Limit(maxExportRows + 1).Find(&rows).Error
	if err != nil {
		return nil, err
	}
	if len(rows) > maxExportRows {
		return nil, ErrExportLimit
	}
	return employeeExportWorkbook(rows, actor.CanReadSalary(), time.Now().In(businessTime))
}

func employeeExportWorkbook(rows []model.Employee, includeSalary bool, now time.Time) ([]byte, error) {
	const sheet = "员工档案"
	f := excelize.NewFile()
	defer f.Close()
	if err := f.SetSheetName("Sheet1", sheet); err != nil {
		return nil, err
	}
	headers := []string{"工号", "姓名", "部门", "出生日期", "年龄", "入职日期", "工龄"}
	if includeSalary {
		headers = append(headers, "约定工资（元）")
	}
	if err := writeExcelTextRow(f, sheet, 1, headers); err != nil {
		return nil, err
	}
	for i, row := range rows {
		view := employeeView(row, includeSalary, now)
		values := []string{
			view.EmployeeNo, view.Name, view.Department, view.BirthDate, strconv.Itoa(view.Age),
			view.HireDate, strconv.Itoa(view.TenureYears),
		}
		if includeSalary {
			// Excel's numeric cells do not preserve every 16-digit cents amount.
			cents := *view.SalaryCents
			values = append(values, fmt.Sprintf("%d.%02d", cents/100, cents%100))
		}
		if err := writeExcelTextRow(f, sheet, i+2, values); err != nil {
			return nil, err
		}
	}
	return workbookBytes(f)
}

func writeExcelTextRow(f *excelize.File, sheet string, row int, values []string) error {
	for index, value := range values {
		cell, err := excelize.CoordinatesToCellName(index+1, row)
		if err != nil {
			return err
		}
		if err := f.SetCellStr(sheet, cell, value); err != nil {
			return err
		}
	}
	return nil
}

func workbookBytes(f *excelize.File) ([]byte, error) {
	buffer, err := f.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func readEmployeeWorkbook(reader io.Reader) ([]model.ImportRow, error) {
	raw, err := io.ReadAll(io.LimitReader(reader, MaxEmployeeWorkbookSize+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > MaxEmployeeWorkbookSize {
		return nil, ErrTooLarge
	}
	if err := auditWorkbookZIP(raw); err != nil {
		return nil, err
	}
	f, err := excelize.OpenReader(bytes.NewReader(raw), excelize.Options{
		RawCellValue: true, UnzipSizeLimit: maxWorkbookExpansion, UnzipXMLSizeLimit: maxWorkbookExpansion,
	})
	if f != nil {
		defer f.Close()
	}
	if err != nil {
		return nil, ErrInvalid
	}
	sheets := f.GetSheetList()
	if len(sheets) != 1 || sheets[0] != importSheetName {
		return nil, ErrInvalid
	}
	iterator, err := f.Rows(importSheetName)
	if err != nil {
		return nil, ErrInvalid
	}
	defer iterator.Close()
	if !iterator.Next() {
		return nil, ErrInvalid
	}
	headers, err := iterator.Columns()
	if err != nil || len(headers) != len(importHeaders) {
		return nil, ErrInvalid
	}
	for i, name := range importHeaders {
		if headers[i] != name {
			return nil, ErrInvalid
		}
	}
	rows := []model.ImportRow{}
	for number := 2; iterator.Next(); number++ {
		if number > maxWorksheetRow {
			return nil, ErrTooLarge
		}
		columns, err := iterator.Columns()
		if err != nil || len(columns) > len(importHeaders) {
			return nil, ErrInvalid
		}
		var cells [6]string
		for index, value := range columns {
			if len(value) > maxImportCellBytes {
				return nil, ErrTooLarge
			}
			cells[index] = strings.TrimSpace(value)
		}
		if cells == [6]string{} {
			continue
		}
		if len(rows) == maxImportRows {
			return nil, ErrTooLarge
		}
		rows = append(rows, model.ImportRow{Row: number, Values: model.ImportValues{
			EmployeeNo: cells[0], Name: cells[1], Department: cells[2],
			BirthDate: cells[3], HireDate: cells[4], Salary: cells[5],
		}, Errors: []model.ImportRowError{}})
	}
	if iterator.Error() != nil {
		return nil, ErrInvalid
	}
	return rows, nil
}

func auditWorkbookZIP(raw []byte) error {
	archive, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return ErrInvalid
	}
	if len(archive.File) > maxWorkbookEntries {
		return ErrTooLarge
	}
	var expanded uint64
	sheets := 0
	seen := make(map[string]bool, len(archive.File))
	for _, entry := range archive.File {
		if entry.UncompressedSize64 > maxWorkbookExpansion-expanded {
			return ErrTooLarge
		}
		expanded += entry.UncompressedSize64
		name := strings.ToLower(strings.TrimSuffix(entry.Name, "/"))
		if name == "" || strings.Contains(name, "\\") || strings.HasPrefix(name, "/") ||
			strings.HasPrefix(name, "../") || name == ".." || path.Clean(name) != name ||
			seen[name] || entry.Flags&1 != 0 ||
			strings.HasPrefix(name, "xl/externallinks/") || strings.HasPrefix(name, "xl/embeddings/") ||
			strings.HasSuffix(name, ".bin") {
			return ErrInvalid
		}
		seen[name] = true
		if entry.FileInfo().IsDir() || !(strings.HasSuffix(name, ".xml") || strings.HasSuffix(name, ".rels")) {
			continue
		}
		reader, err := entry.Open()
		if err != nil {
			return ErrInvalid
		}
		worksheet, err := auditWorkbookXML(reader)
		closeErr := reader.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return ErrInvalid
		}
		if worksheet {
			sheets++
		}
	}
	if sheets != 1 {
		return ErrInvalid
	}
	return nil
}

func auditWorkbookXML(reader io.Reader) (bool, error) {
	decoder := xml.NewDecoder(reader)
	sheet, started := false, false
	row := 0
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			return sheet, nil
		}
		if err != nil {
			return false, ErrInvalid
		}
		switch element := token.(type) {
		case xml.Directive:
			return false, ErrInvalid
		case xml.StartElement:
			if !started {
				sheet, started = element.Name.Local == "worksheet", true
			}
			if err := auditWorkbookElement(element, sheet, &row); err != nil {
				return false, err
			}
		}
	}
}

func auditWorkbookElement(element xml.StartElement, sheet bool, row *int) error {
	for _, attr := range element.Attr {
		value := strings.ToLower(attr.Value)
		if (attr.Name.Local == "TargetMode" && value == "external") ||
			(attr.Name.Local == "ContentType" && (strings.Contains(value, "macroenabled") || strings.Contains(value, "vbaproject"))) {
			return ErrInvalid
		}
	}
	if element.Name.Local == "externalLink" || element.Name.Local == "externalReferences" || element.Name.Local == "oleObjects" {
		return ErrInvalid
	}
	if !sheet {
		return nil
	}
	switch element.Name.Local {
	case "f", "formula", "formula1", "formula2":
		return ErrInvalid
	case "row":
		number := *row + 1
		for _, attr := range element.Attr {
			if attr.Name.Local == "r" {
				declared, err := strconv.Atoi(attr.Value)
				if err != nil || declared < number {
					return ErrInvalid
				}
				number = declared
			}
		}
		if number > maxWorksheetRow {
			return ErrTooLarge
		}
		*row = number
	case "c", "dimension":
		for _, attr := range element.Attr {
			if (element.Name.Local == "c" && attr.Name.Local == "r") ||
				(element.Name.Local == "dimension" && attr.Name.Local == "ref") {
				for _, cell := range strings.Split(attr.Value, ":") {
					column, number, err := excelize.CellNameToCoordinates(cell)
					if err != nil || column > len(importHeaders) {
						return ErrInvalid
					}
					if number > maxWorksheetRow {
						return ErrTooLarge
					}
					if element.Name.Local == "c" && number != *row {
						return ErrInvalid
					}
				}
			}
		}
	}
	return nil
}
