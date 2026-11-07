package service

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"strings"

	system "github.com/flipped-aurora/gin-vue-admin/server/model/system"
	"github.com/flipped-aurora/gin-vue-admin/server/oa/model"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrInvalidState    = errors.New("invalid lifecycle state")
	ErrAccountConflict = errors.New("account conflict")
	ErrHandoffRequired = errors.New("handoff required")
	ErrHandoffConflict = errors.New("handoff conflict")
	accountUsername    = regexp.MustCompile(`^[A-Za-z0-9._-]{3,64}$`)
)

func managedRole(role string) uint {
	switch role {
	case "EMPLOYEE":
		return model.EmployeeRole
	case "APPROVER":
		return model.ApproverRole
	default:
		return 0
	}
}

func validateAccountAction(input model.AccountAction) (string, uint, error) {
	reason, err := auditReason(input.Reason, "")
	if err != nil || input.Version < 1 {
		return "", 0, ErrInvalid
	}
	var authority uint
	if input.Role != nil {
		authority = managedRole(*input.Role)
		if authority == 0 {
			return "", 0, ErrInvalid
		}
	}
	if input.Password != nil && (len(*input.Password) < 12 || len(*input.Password) > 72 ||
		strings.ContainsRune(*input.Password, '\x00') || strings.Contains(reason, *input.Password)) {
		return "", 0, ErrInvalid
	}
	if (input.UserID != nil && *input.UserID == 0) || (input.HandoffUserID != nil && *input.HandoffUserID == 0) {
		return "", 0, ErrInvalid
	}
	if (input.Username != nil) != (input.Action == "CREATE") ||
		(input.Password != nil) != (input.Action == "CREATE" || input.Action == "RESET_PASSWORD") ||
		(input.Role != nil) != (input.Action == "CREATE" || input.Action == "SET_ROLE") ||
		(input.UserID != nil) != (input.Action == "BIND") ||
		(input.HandoffUserID != nil && input.Action != "SET_ROLE" && input.Action != "DISABLE" && input.Action != "OFFBOARD") {
		return "", 0, ErrInvalid
	}
	switch input.Action {
	case "CREATE":
		if !accountUsername.MatchString(*input.Username) {
			return "", 0, ErrInvalid
		}
	case "BIND", "SET_ROLE", "RESET_PASSWORD", "DISABLE", "ENABLE", "OFFBOARD":
	default:
		return "", 0, ErrInvalid
	}
	return reason, authority, nil
}

func accountEmployee(db *gorm.DB, employeeID uint) (model.Employee, error) {
	var employee model.Employee
	err := db.Select("id", "name", "user_id", "version", "employment_status").Take(&employee, employeeID).Error
	return employee, queryError(err)
}

func accountUser(db *gorm.DB, id uint) (system.SysUser, error) {
	var user system.SysUser
	err := db.Select("id", "username", "authority_id", "enable").Take(&user, id).Error
	return user, queryError(err)
}

func accountDetails(user system.SysUser) model.AccountDetails {
	return model.AccountDetails{ID: user.ID, Username: user.Username, Role: model.RoleName(user.AuthorityId), Enabled: user.Enable == 1}
}

func employeeAccount(employee model.Employee, user system.SysUser) model.EmployeeAccount {
	result := model.EmployeeAccount{EmployeeID: employee.ID, EmploymentStatus: employee.EmploymentStatus, Version: employee.Version}
	if user.ID != 0 {
		details := accountDetails(user)
		result.Account = &details
	}
	return result
}

func (s *Service) EmployeeAccount(ctx context.Context, actor model.Actor, employeeID uint) (model.EmployeeAccount, error) {
	if !actor.CanWrite() {
		return model.EmployeeAccount{}, ErrForbidden
	}
	var result model.EmployeeAccount
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		employee, err := accountEmployee(tx, employeeID)
		if err != nil {
			return err
		}
		var user system.SysUser
		if employee.UserID != nil {
			user, err = accountUser(tx, *employee.UserID)
			if err != nil {
				return err
			}
		}
		result = employeeAccount(employee, user)
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	return result, err
}

func (s *Service) AccountCandidates(ctx context.Context, actor model.Actor, employeeID uint, p model.Pagination) (model.Page[model.AccountDetails], error) {
	result := model.NewPage[model.AccountDetails](p)
	if !actor.CanWrite() {
		return result, ErrForbidden
	}
	db := s.db.WithContext(ctx)
	if _, err := accountEmployee(db, employeeID); err != nil {
		return result, err
	}
	query := db.Model(&system.SysUser{}).
		Where("authority_id IN ? AND id <> ?", []uint{model.EmployeeRole, model.ApproverRole}, actor.UserID).
		Where("NOT EXISTS (SELECT 1 FROM oa_employees AS e WHERE e.user_id = sys_users.id)")
	if err := query.Count(&result.Total).Error; err != nil {
		return result, err
	}
	var users []system.SysUser
	err := query.Select("id", "username", "authority_id", "enable").Order("id ASC").
		Limit(p.PageSize).Offset(p.Offset()).Find(&users).Error
	for _, user := range users {
		result.List = append(result.List, accountDetails(user))
	}
	return result, err
}

func (s *Service) AccountHandoffTargets(ctx context.Context, actor model.Actor, employeeID uint, p model.Pagination) (model.Page[model.TransferTarget], error) {
	result := model.NewPage[model.TransferTarget](p)
	if !actor.CanWrite() {
		return result, ErrForbidden
	}
	db := s.db.WithContext(ctx)
	employee, err := accountEmployee(db, employeeID)
	if err != nil {
		return result, err
	}
	query := boundWorkflowUsers(db).Where("u.authority_id = ?", model.ApproverRole).
		Where("NOT EXISTS (SELECT 1 FROM oa_approval_handoffs AS h WHERE h.user_id = u.id)")
	if employee.UserID != nil {
		query = query.Where("u.id <> ?", *employee.UserID)
	}
	if err := query.Count(&result.Total).Error; err != nil {
		return result, err
	}
	err = query.Select("u.id, u.nick_name AS display_name").Order("u.id ASC").
		Limit(p.PageSize).Offset(p.Offset()).Scan(&result.List).Error
	return result, err
}

func (s *Service) ActOnAccount(ctx context.Context, actor model.Actor, employeeID uint, input model.AccountAction) (model.EmployeeAccount, error) {
	if !actor.CanWrite() {
		return model.EmployeeAccount{}, ErrForbidden
	}
	reason, authority, err := validateAccountAction(input)
	if err != nil {
		return model.EmployeeAccount{}, err
	}
	var hash []byte
	if input.Password != nil {
		hash, err = bcrypt.GenerateFromPassword([]byte(*input.Password), bcrypt.DefaultCost)
		if err != nil {
			return model.EmployeeAccount{}, err
		}
	}
	var result model.EmployeeAccount
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		tx = s.notificationTx(tx)
		if err := lockWorkflow(tx, "UPDATE"); err != nil {
			return err
		}
		lock := clause.Locking{Strength: "UPDATE"}
		employee, err := accountEmployee(tx.Clauses(lock), employeeID)
		if err != nil {
			return err
		}
		if employee.Version != input.Version {
			return ErrStaleRecord
		}
		if employee.EmploymentStatus != model.EmploymentActive {
			return ErrInvalidState
		}
		var user system.SysUser
		if employee.UserID != nil {
			user, err = accountUser(tx.Clauses(lock), *employee.UserID)
			if err != nil {
				return err
			}
			if err := checkManagedAccount(tx, actor, user); err != nil {
				return err
			}
		}
		before := accountAuditFields(user)
		if err := applyAccountAction(tx, actor, &employee, &user, input, authority, hash); err != nil {
			return err
		}
		if user.ID != 0 {
			if err := revokeSessions(tx, user.ID); err != nil {
				return err
			}
			after := accountAuditFields(user)
			if input.HandoffUserID != nil {
				after["handoffUserId"] = *input.HandoffUserID
			}
			if err := writeAudit(tx, actor, model.AuditEvent{
				Type: model.AuditAccount, Action: input.Action, Reason: reason, EmployeeID: &employeeID,
			}, before, after); err != nil {
				return err
			}
		}
		if input.Action == "OFFBOARD" {
			employee.EmploymentStatus = model.EmploymentOffboarded
			if err := writeAudit(tx, actor, model.AuditEvent{
				Type: model.AuditEmployment, Action: input.Action, Reason: reason, EmployeeID: &employeeID,
			}, auditFields{"employmentStatus": model.EmploymentActive},
				auditFields{"employmentStatus": model.EmploymentOffboarded}); err != nil {
				return err
			}
		}
		employee.Version++
		if err := tx.Model(&model.Employee{}).Where("id = ?", employee.ID).Updates(map[string]any{
			"user_id": employee.UserID, "version": employee.Version, "employment_status": employee.EmploymentStatus,
		}).Error; err != nil {
			if errors.Is(createError(err), ErrConflict) {
				return ErrAccountConflict
			}
			return err
		}
		result = employeeAccount(employee, user)
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	return result, recordWriteError(err)
}

func checkManagedAccount(tx *gorm.DB, actor model.Actor, user system.SysUser) error {
	if user.ID == actor.UserID || (user.AuthorityId != model.EmployeeRole && user.AuthorityId != model.ApproverRole) {
		return ErrForbidden
	}
	var count int64
	err := tx.Table("sys_user_authority").Where("sys_user_id = ? AND sys_authority_authority_id NOT IN ?",
		user.ID, []uint{model.EmployeeRole, model.ApproverRole}).Count(&count).Error
	if err == nil && count != 0 {
		return ErrForbidden
	}
	return err
}

func applyAccountAction(tx *gorm.DB, actor model.Actor, employee *model.Employee, user *system.SysUser,
	input model.AccountAction, authority uint, hash []byte,
) error {
	if input.Action == "CREATE" || input.Action == "BIND" {
		if employee.UserID != nil {
			return ErrAccountConflict
		}
		if input.Action == "CREATE" {
			created, err := createEmployeeAccount(tx, *input.Username, employee.Name, authority, hash)
			if err != nil {
				return err
			}
			*user = created
		} else {
			candidate, err := accountUser(tx.Clauses(clause.Locking{Strength: "UPDATE"}), *input.UserID)
			if err != nil {
				return err
			}
			if err := checkManagedAccount(tx, actor, candidate); err != nil {
				return err
			}
			var count int64
			if err := tx.Unscoped().Model(&model.Employee{}).Where("user_id = ?", candidate.ID).Count(&count).Error; err != nil {
				return err
			}
			if count != 0 {
				return ErrAccountConflict
			}
			*user = candidate
		}
		employee.UserID = &user.ID
		return nil
	}
	if user.ID == 0 {
		if input.Action == "OFFBOARD" && input.HandoffUserID == nil {
			return nil
		}
		return ErrInvalidState
	}
	if input.Action == "ENABLE" && user.Enable == 1 || input.Action == "DISABLE" && user.Enable != 1 ||
		input.Action == "SET_ROLE" && user.AuthorityId == authority {
		return ErrInvalidState
	}
	removesApprover := user.AuthorityId == model.ApproverRole &&
		(input.Action == "DISABLE" || input.Action == "OFFBOARD" || input.Action == "SET_ROLE")
	if removesApprover {
		if err := handoffApprovals(tx, actor, *user, input.HandoffUserID); err != nil {
			return err
		}
	} else if input.HandoffUserID != nil {
		return ErrInvalid
	}

	updates := make(map[string]any)
	switch input.Action {
	case "SET_ROLE":
		if err := setMembership(tx, user.ID, authority); err != nil {
			return err
		}
		user.AuthorityId = authority
		updates["authority_id"] = authority
	case "RESET_PASSWORD":
		updates["password"] = string(hash)
	case "ENABLE":
		user.Enable, updates["enable"] = 1, 1
	case "DISABLE", "OFFBOARD":
		user.Enable, updates["enable"] = 2, 2
	}
	return tx.Model(&system.SysUser{}).Where("id = ?", user.ID).Updates(updates).Error
}

func createEmployeeAccount(tx *gorm.DB, username, name string, authority uint, hash []byte) (system.SysUser, error) {
	var count int64
	if err := tx.Unscoped().Model(&system.SysUser{}).Where("username = ?", username).Count(&count).Error; err != nil {
		return system.SysUser{}, err
	}
	if count != 0 {
		return system.SysUser{}, ErrAccountConflict
	}
	identifier, err := uuid.NewRandom()
	if err != nil {
		return system.SysUser{}, err
	}
	user := system.SysUser{UUID: identifier, Username: username, NickName: name, Password: string(hash), AuthorityId: authority, Enable: 1}
	if err := tx.Create(&user).Error; err != nil {
		return system.SysUser{}, err
	}
	return user, setMembership(tx, user.ID, authority)
}

func setMembership(tx *gorm.DB, id, authority uint) error {
	var role system.SysAuthority
	if err := tx.Select("authority_id").Take(&role, "authority_id = ? AND deleted_at IS NULL", authority).Error; err != nil {
		return queryError(err)
	}
	if err := tx.Exec("DELETE FROM sys_user_authority WHERE sys_user_id = ?", id).Error; err != nil {
		return err
	}
	return tx.Exec("INSERT INTO sys_user_authority (sys_user_id, sys_authority_authority_id) VALUES (?, ?)", id, authority).Error
}

func accountAuditFields(user system.SysUser) auditFields {
	if user.ID == 0 {
		return nil
	}
	return auditFields{"userId": user.ID, "role": model.RoleName(user.AuthorityId), "enabled": user.Enable == 1}
}

func revokeSessions(tx *gorm.DB, id uint) error {
	return tx.Clauses(clause.OnConflict{DoUpdates: clause.Assignments(map[string]any{"epoch": gorm.Expr("epoch + 1")})}).
		Create(&model.AccountSession{UserID: id, Epoch: 1}).Error
}
