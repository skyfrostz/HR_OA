package service

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/flipped-aurora/gin-vue-admin/server/oa/model"
	"github.com/xuri/excelize/v2"
)

func (s *Service) ImportHistory(ctx context.Context, actor model.Actor, p model.Pagination) (model.Page[model.ImportHistoryItem], error) {
	page := model.NewPage[model.ImportHistoryItem](p)
	if !actor.CanWrite() {
		return page, ErrForbidden
	}
	now := workflowNow()
	query := s.db.WithContext(ctx).Model(&model.EmployeeImport{}).Where("creator_id = ?", actor.UserID)
	if err := query.Count(&page.Total).Error; err != nil {
		return page, err
	}
	err := query.Select(`id, created_at, expires_at, confirmed_at,
		CASE WHEN status = 'PREVIEW' AND expires_at <= ? THEN 'EXPIRED' ELSE status END AS status,
		CAST(JSON_UNQUOTE(JSON_EXTRACT(preview, '$.totalRows')) AS UNSIGNED) AS total_rows,
		CAST(JSON_UNQUOTE(JSON_EXTRACT(preview, '$.validRows')) AS UNSIGNED) AS valid_rows,
		CAST(JSON_UNQUOTE(JSON_EXTRACT(preview, '$.errorRows')) AS UNSIGNED) AS error_rows,
		CAST(JSON_UNQUOTE(JSON_EXTRACT(result, '$.importedCount')) AS UNSIGNED) AS imported_count`, now).
		Order("created_at DESC, id DESC").Limit(p.PageSize).Offset(p.Offset()).Scan(&page.List).Error
	return page, err
}

func (s *Service) EmployeeImportResult(ctx context.Context, actor model.Actor, id string) (model.ImportResult, error) {
	var result model.ImportResult
	if !actor.CanWrite() {
		return result, ErrForbidden
	}
	if !validPreviewID(id) {
		return result, ErrNotFound
	}
	var record model.EmployeeImport
	if err := s.db.WithContext(ctx).Select("status", "expires_at", "result").
		Take(&record, "id = ? AND creator_id = ?", id, actor.UserID).Error; err != nil {
		return result, queryError(err)
	}
	if record.Status != model.ImportCommitted {
		return result, ErrInvalidState
	}
	err := json.Unmarshal(record.Result, &result)
	return result, err
}

func (s *Service) EmployeeImportErrors(ctx context.Context, actor model.Actor, id string) ([]byte, error) {
	preview, err := s.EmployeeImportPreview(ctx, actor, id)
	if err != nil {
		return nil, err
	}
	if preview.Status == model.ImportExpired {
		return nil, ErrPreviewExpired
	}
	if preview.Status != model.ImportPending {
		return nil, ErrInvalidState
	}
	return importErrorWorkbook(preview.Rows)
}

func importErrorWorkbook(rows []model.ImportRow) ([]byte, error) {
	const sheet = "导入错误"
	workbook := excelize.NewFile()
	defer workbook.Close()
	if err := workbook.SetSheetName("Sheet1", sheet); err != nil {
		return nil, err
	}
	headers := []string{"行号", "工号", "姓名", "部门", "出生日期", "入职日期", "约定工资（元）", "错误"}
	if err := writeExcelTextRow(workbook, sheet, 1, headers); err != nil {
		return nil, err
	}
	rowNumber := 2
	for _, row := range rows {
		if len(row.Errors) == 0 {
			continue
		}
		messages := make([]string, 0, len(row.Errors))
		for _, issue := range row.Errors {
			messages = append(messages, issue.Field+"："+issue.Message)
		}
		values := row.Values
		if err := writeExcelTextRow(workbook, sheet, rowNumber, []string{strconv.Itoa(row.Row), values.EmployeeNo,
			values.Name, values.Department, values.BirthDate, values.HireDate, values.Salary, strings.Join(messages, "；")}); err != nil {
			return nil, err
		}
		rowNumber++
	}
	return workbookBytes(workbook)
}

func (s *Service) RunCleanup(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for ctx.Err() == nil {
		attempt, cancel := context.WithTimeout(ctx, 5*time.Second)
		_ = s.cleanup(attempt)
		cancel()
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Service) cleanup(ctx context.Context) error {
	now := workflowNow()
	db := s.db.WithContext(ctx)
	err := db.Exec(`UPDATE oa_employee_imports
		SET preview = JSON_SET(preview,'$.rows',JSON_ARRAY(),'$.canConfirm',CAST('false' AS JSON)),
		    validated = NULL, status = IF(status = 'PREVIEW','EXPIRED',status), purged_at = ?
		WHERE purged_at IS NULL AND (status = 'COMMITTED' OR (status = 'PREVIEW' AND expires_at <= ?))
		ORDER BY expires_at, id LIMIT 100`, now, now).Error
	return errors.Join(err, s.cleanupWeCom(ctx, now))
}
