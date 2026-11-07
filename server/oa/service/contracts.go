package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/flipped-aurora/gin-vue-admin/server/oa/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (s *Service) visibleContracts(ctx context.Context, actor model.Actor) *gorm.DB {
	employees := s.visibleEmployees(ctx, actor).Select("oa_employees.id")
	return s.db.WithContext(ctx).Model(&model.Contract{}).Where("employee_id IN (?)", employees)
}

func (s *Service) ListContracts(ctx context.Context, actor model.Actor, employeeID uint, p model.Pagination) (model.Page[model.ContractView], error) {
	page := model.NewPage[model.ContractView](p)
	var employee model.Employee
	if err := s.visibleEmployees(ctx, actor).Select("id").Take(&employee, employeeID).Error; err != nil {
		return page, queryError(err)
	}
	query := s.visibleContracts(ctx, actor).Where("employee_id = ?", employeeID)
	if err := query.Count(&page.Total).Error; err != nil {
		return page, err
	}
	var contracts []model.Contract
	err := query.Order("id DESC").Limit(p.PageSize).Offset(p.Offset()).
		Preload("Files", func(db *gorm.DB) *gorm.DB { return scopedFiles(db, actor) }).Find(&contracts).Error
	for _, contract := range contracts {
		page.List = append(page.List, contractView(contract))
	}
	return page, err
}

func (s *Service) CreateContract(ctx context.Context, actor model.Actor, employeeID uint, input model.ContractInput) (model.ContractView, error) {
	if !actor.CanWrite() {
		return model.ContractView{}, ErrForbidden
	}

	number := strings.TrimSpace(input.ContractNo)
	start, startErr := parseDate(input.StartDate)
	end, endErr := parseDate(input.EndDate)
	if !validText(number, 64) || startErr != nil || endErr != nil || start.After(end) || !validContractStatus(input.Status) {
		return model.ContractView{}, ErrInvalid
	}

	reason, err := auditReason(input.Reason, "登记合同")
	if err != nil {
		return model.ContractView{}, err
	}
	contract := model.Contract{
		EmployeeID: employeeID, ContractNo: number, StartDate: start, EndDate: end, Status: input.Status,
		PreviousContractID: input.PreviousContractID, Version: 1,
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var employee model.Employee
		if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).Select("id").Take(&employee, employeeID).Error; err != nil {
			return queryError(err)
		}
		action := "CREATE"
		if input.PreviousContractID != nil {
			var prior model.Contract
			err := tx.Clauses(clause.Locking{Strength: "SHARE"}).Select("id").
				Take(&prior, "id = ? AND employee_id = ?", *input.PreviousContractID, employeeID).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrInvalid
			}
			if err != nil {
				return err
			}
			action = "RENEW"
		}
		if err := createError(tx.Create(&contract).Error); err != nil {
			return err
		}
		return writeContractAudit(tx, actor, action, reason, nil, contract)
	})
	return contractView(contract), recordWriteError(err)
}

func (s *Service) Contract(ctx context.Context, actor model.Actor, id uint) (model.ContractView, error) {
	var contract model.Contract
	err := s.visibleContracts(ctx, actor).Preload("Files", func(db *gorm.DB) *gorm.DB {
		return scopedFiles(db, actor)
	}).Take(&contract, id).Error
	return contractView(contract), queryError(err)
}

func (s *Service) EditContract(ctx context.Context, actor model.Actor, id uint, input model.ContractEdit) (model.ContractView, error) {
	number := strings.TrimSpace(input.ContractNo)
	start, startErr := parseDate(input.StartDate)
	end, endErr := parseDate(input.EndDate)
	if !validText(number, 64) || startErr != nil || endErr != nil || start.After(end) {
		return model.ContractView{}, ErrInvalid
	}
	return s.changeContract(ctx, actor, id, input.Version, "UPDATE", input.Reason, func(row *model.Contract) (map[string]any, error) {
		if row.Status != "DRAFT" {
			return nil, ErrInvalidState
		}
		return map[string]any{"contract_no": number, "start_date": start, "end_date": end}, nil
	})
}

func (s *Service) ActOnContract(ctx context.Context, actor model.Actor, id uint, input model.ContractAction) (model.ContractView, error) {
	date, err := parseDate(input.EffectiveDate)
	if err != nil || input.EffectiveDate > time.Now().In(businessTime).Format(dateLayout) ||
		(input.Action != "SIGN" && input.Action != "TERMINATE") {
		return model.ContractView{}, ErrInvalid
	}
	return s.changeContract(ctx, actor, id, input.Version, input.Action, input.Reason, func(row *model.Contract) (map[string]any, error) {
		return contractTransition(*row, input.Action, date)
	})
}

func contractTransition(row model.Contract, action string, date time.Time) (map[string]any, error) {
	switch action {
	case "SIGN":
		if row.Status != "DRAFT" {
			return nil, ErrInvalidState
		}
		return map[string]any{"status": "SIGNED", "signed_date": date}, nil
	case "TERMINATE":
		if row.Status != "SIGNED" {
			return nil, ErrInvalidState
		}
		if date.Before(row.StartDate) || (row.SignedDate != nil && date.Before(*row.SignedDate)) {
			return nil, ErrInvalid
		}
		return map[string]any{"status": "TERMINATED", "terminated_date": date}, nil
	default:
		return nil, ErrInvalid
	}
}

func (s *Service) changeContract(ctx context.Context, actor model.Actor, id uint, version int, action, reason string,
	change func(*model.Contract) (map[string]any, error),
) (model.ContractView, error) {
	if !actor.CanWrite() {
		return model.ContractView{}, ErrForbidden
	}
	reason, err := auditReason(reason, "")
	if err != nil || version < 1 {
		return model.ContractView{}, ErrInvalid
	}
	var contract model.Contract
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var before model.Contract
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Take(&before, id).Error; err != nil {
			return queryError(err)
		}
		if before.Version != version {
			return ErrStaleRecord
		}
		fields, err := change(&before)
		if err != nil {
			return err
		}
		fields["version"] = version + 1
		if err := createError(tx.Model(&model.Contract{}).Where("id = ?", id).Updates(fields).Error); err != nil {
			return err
		}
		if err := tx.Preload("Files", func(db *gorm.DB) *gorm.DB { return scopedFiles(db, actor) }).Take(&contract, id).Error; err != nil {
			return err
		}
		return writeContractAudit(tx, actor, action, reason, &before, contract)
	})
	return contractView(contract), recordWriteError(err)
}

func writeContractAudit(tx *gorm.DB, actor model.Actor, action, reason string, before *model.Contract, after model.Contract) error {
	var old auditFields
	if before != nil {
		old = contractAuditFields(*before)
	}
	return writeAudit(tx, actor, model.AuditEvent{
		Type: model.AuditContract, Action: action, Reason: reason, EmployeeID: &after.EmployeeID, ContractID: &after.ID,
	}, old, contractAuditFields(after))
}

func contractAuditFields(row model.Contract) auditFields {
	return auditFields{
		"contractNo": row.ContractNo, "startDate": row.StartDate.Format(dateLayout), "endDate": row.EndDate.Format(dateLayout),
		"status": row.Status, "version": row.Version, "previousContractId": row.PreviousContractID,
		"signedDate": contractDate(row.SignedDate), "terminatedDate": contractDate(row.TerminatedDate),
	}
}

func contractDate(date *time.Time) *string {
	if date == nil {
		return nil
	}
	value := date.Format(dateLayout)
	return &value
}

func validContractStatus(status string) bool {
	switch status {
	case "DRAFT", "SIGNED", "EXPIRED", "TERMINATED":
		return true
	default:
		return false
	}
}

func contractView(contract model.Contract) model.ContractView {
	view := model.ContractView{
		ID: contract.ID, EmployeeID: contract.EmployeeID, ContractNo: contract.ContractNo, Status: contract.Status,
		StartDate: contract.StartDate.Format(dateLayout), EndDate: contract.EndDate.Format(dateLayout),
		Version: contract.Version, SignedDate: contractDate(contract.SignedDate), TerminatedDate: contractDate(contract.TerminatedDate),
		PreviousContractID: contract.PreviousContractID, Files: make([]model.FileView, 0, len(contract.Files)),
	}
	for _, file := range contract.Files {
		view.Files = append(view.Files, fileView(file))
	}
	return view
}

func fileView(file model.ContractFile) model.FileView {
	return model.FileView{
		ID: file.ID, OriginalName: file.OriginalName, Size: file.Size, ContentType: file.ContentType,
		Classification: file.Classification, Version: file.Version,
		CreatedAt: file.CreatedAt.UTC(),
	}
}
