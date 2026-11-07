package service

import (
	"context"
	"errors"
	"strings"

	"github.com/flipped-aurora/gin-vue-admin/server/oa/model"
	"gorm.io/gorm"
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
		Preload("Files", func(db *gorm.DB) *gorm.DB { return db.Order("id ASC") }).Find(&contracts).Error
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

	var employee model.Employee
	if err := s.visibleEmployees(ctx, actor).Select("id").Take(&employee, employeeID).Error; err != nil {
		return model.ContractView{}, queryError(err)
	}
	if input.PreviousContractID != nil {
		if *input.PreviousContractID == 0 {
			return model.ContractView{}, ErrInvalid
		}
		var prior model.Contract
		err := s.visibleContracts(ctx, actor).Select("id").Take(&prior, "id = ? AND employee_id = ?", *input.PreviousContractID, employeeID).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return model.ContractView{}, ErrInvalid
		}
		if err != nil {
			return model.ContractView{}, err
		}
	}

	contract := model.Contract{
		EmployeeID: employeeID, ContractNo: number, StartDate: start, EndDate: end, Status: input.Status,
		PreviousContractID: input.PreviousContractID,
	}
	if err := s.db.WithContext(ctx).Create(&contract).Error; err != nil {
		return model.ContractView{}, createError(err)
	}
	return contractView(contract), nil
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
		CreatedAt: file.CreatedAt.UTC(),
	}
}
