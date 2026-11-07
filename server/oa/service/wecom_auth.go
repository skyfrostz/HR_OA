package service

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/flipped-aurora/gin-vue-admin/server/oa/integration"
	"github.com/flipped-aurora/gin-vue-admin/server/oa/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrIntegrationDisabled = errors.New("integration disabled")

const wecomStateLifetime = 5 * time.Minute
const wecomTicketLifetime = 90 * time.Second

func (s *Service) WeComOrigin() string {
	if s.wecom == nil {
		return ""
	}
	return s.wecom.Origin()
}

func (s *Service) createWeComChallenge(ctx context.Context, challenge model.WeComChallenge, life time.Duration) (string, error) {
	var key [32]byte
	if _, err := rand.Read(key[:]); err != nil {
		return "", err
	}
	secret := hex.EncodeToString(key[:])
	challenge.Hash, challenge.CorpID, challenge.ExpiresAt = digest([]byte(secret)), s.wecom.CorpID(), workflowNow().Add(life)
	challenge.Mode = s.wecom.Mode()
	if err := s.db.WithContext(ctx).Create(&challenge).Error; err != nil {
		return "", err
	}
	return secret, nil
}

func (s *Service) StartWeCom(ctx context.Context, entry string) (string, string, error) {
	if s.wecom == nil {
		return "", "", ErrIntegrationDisabled
	}
	if entry != "pc" && entry != "h5" {
		return "", "", ErrInvalid
	}
	state, err := s.createWeComChallenge(ctx, model.WeComChallenge{Kind: "STATE", Entry: entry}, wecomStateLifetime)
	if err != nil {
		return "", "", err
	}
	return s.wecom.AuthorizationURL(state, entry), state, nil
}

func opaqueWeComSecret(value string) bool {
	if len(value) != 64 || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func (s *Service) consumeWeComChallenge(ctx context.Context, secret, kind string) (model.WeComChallenge, error) {
	var challenge model.WeComChallenge
	if !opaqueWeComSecret(secret) {
		return challenge, ErrUnauthorized
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Take(&challenge, "hash = ? AND mode = ? AND corp_id = ? AND kind = ?", digest([]byte(secret)), s.wecom.Mode(), s.wecom.CorpID(), kind).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrUnauthorized
		}
		if err != nil {
			return err
		}
		if !workflowNow().Before(challenge.ExpiresAt) {
			return ErrUnauthorized
		}
		return tx.Delete(&challenge).Error
	}, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	return challenge, err
}

func (s *Service) CompleteWeCom(ctx context.Context, state, cookie, code string) (string, string, error) {
	if s.wecom == nil {
		return "", "", ErrIntegrationDisabled
	}
	if !opaqueWeComSecret(state) || subtle.ConstantTimeCompare([]byte(state), []byte(cookie)) != 1 {
		return "", "", ErrUnauthorized
	}
	challenge, err := s.consumeWeComChallenge(ctx, state, "STATE")
	if err != nil {
		return challenge.Entry, "", err
	}
	member, err := s.wecom.Member(ctx, code)
	if err != nil {
		return challenge.Entry, "", ErrUnauthorized
	}
	var user loginAccount
	err = s.wecomIdentity(ctx).Take(&user, "w.member_id = ?", member).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = ErrUnauthorized
	}
	if err != nil {
		return challenge.Entry, "", err
	}
	ticket, err := s.createWeComChallenge(ctx, model.WeComChallenge{
		Kind: "TICKET", Entry: challenge.Entry, UserID: user.ID, SessionEpoch: user.SessionEpoch,
	}, wecomTicketLifetime)
	return challenge.Entry, ticket, err
}

func (s *Service) wecomIdentity(ctx context.Context) *gorm.DB {
	return identityQuery(s.db.WithContext(ctx)).
		Select("sys_users.id, sys_users.uuid, sys_users.username, sys_users.nick_name, sys_users.authority_id, sys_users.enable, COALESCE(sessions.epoch, 0) AS session_epoch").
		Joins("JOIN oa_wecom_bindings AS w ON w.user_id = sys_users.id AND w.mode = ? AND w.corp_id = ?", s.wecom.Mode(), s.wecom.CorpID()).
		Joins("JOIN oa_employees AS e ON e.id = w.employee_id AND e.user_id = sys_users.id AND e.employment_status = ? AND e.deleted_at IS NULL", model.EmploymentActive).
		Where("sys_users.enable = ? AND sys_users.authority_id IN ?", 1, []uint{model.EmployeeRole, model.ApproverRole}).
		Where("EXISTS (SELECT 1 FROM sys_authorities AS a WHERE a.authority_id = sys_users.authority_id AND a.deleted_at IS NULL)")
}

func (s *Service) ExchangeWeCom(ctx context.Context, ticket string) (model.LoginView, error) {
	if s.wecom == nil {
		return model.LoginView{}, ErrIntegrationDisabled
	}
	challenge, err := s.consumeWeComChallenge(ctx, ticket, "TICKET")
	if err != nil {
		return model.LoginView{}, err
	}
	var user loginAccount
	err = s.wecomIdentity(ctx).Take(&user, "sys_users.id = ? AND COALESCE(sessions.epoch, 0) = ?", challenge.UserID, challenge.SessionEpoch).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = ErrUnauthorized
	}
	if err != nil {
		return model.LoginView{}, err
	}
	return s.loginView(ctx, user)
}

func (s *Service) WeComBinding(ctx context.Context, actor model.Actor, employeeID uint) (model.WeComBindingView, error) {
	var result model.WeComBindingView
	if !actor.CanWrite() {
		return result, ErrForbidden
	}
	db := s.db.WithContext(ctx)
	employee, err := accountEmployee(db, employeeID)
	if err != nil {
		return result, err
	}
	result.EmployeeID, result.Version = employee.ID, employee.Version
	if s.wecom == nil {
		return result, nil
	}
	var binding model.WeComBinding
	err = db.Take(&binding, "employee_id = ? AND mode = ? AND corp_id = ?", employeeID, s.wecom.Mode(), s.wecom.CorpID()).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return result, nil
	}
	if err == nil {
		result.MemberID = &binding.MemberID
	}
	return result, err
}

func (s *Service) WriteWeComBinding(ctx context.Context, actor model.Actor, employeeID uint, input model.WeComBindingInput) (model.WeComBindingView, error) {
	var result model.WeComBindingView
	if !actor.CanWrite() {
		return result, ErrForbidden
	}
	if s.wecom == nil {
		return result, ErrIntegrationDisabled
	}
	reason, err := auditReason(input.Reason, "")
	if err != nil || input.Version < 1 {
		return result, ErrInvalid
	}
	if input.MemberID != nil {
		member, ok := integration.NormalizeMemberID(*input.MemberID)
		if !ok {
			return result, ErrInvalid
		}
		input.MemberID = &member
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockWorkflow(tx, "UPDATE"); err != nil {
			return err
		}
		employee, err := accountEmployee(tx.Clauses(clause.Locking{Strength: "UPDATE"}), employeeID)
		if err != nil {
			return err
		}
		if employee.Version != input.Version {
			return ErrStaleRecord
		}
		if employee.UserID == nil || employee.EmploymentStatus != model.EmploymentActive {
			return ErrInvalidState
		}
		user, err := accountUser(tx.Clauses(clause.Locking{Strength: "UPDATE"}), *employee.UserID)
		if err != nil {
			return err
		}
		if err := checkManagedAccount(tx, actor, user); err != nil {
			return err
		}
		var old model.WeComBinding
		err = tx.Take(&old, employeeID).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		bound := old.EmployeeID != 0
		current := bound && old.Mode == s.wecom.Mode() && old.CorpID == s.wecom.CorpID()
		action := "BIND"
		if input.MemberID == nil {
			action = "UNBIND"
			if !current {
				return ErrInvalidState
			}
			if err := tx.Delete(&old).Error; err != nil {
				return err
			}
		} else {
			if current && old.MemberID == *input.MemberID {
				return ErrInvalidState
			}
			binding := model.WeComBinding{EmployeeID: employeeID, UserID: user.ID, Mode: s.wecom.Mode(), CorpID: s.wecom.CorpID(), MemberID: *input.MemberID}
			if bound {
				err = tx.Model(&old).Updates(map[string]any{"mode": binding.Mode, "corp_id": binding.CorpID, "member_id": binding.MemberID, "user_id": binding.UserID}).Error
			} else {
				err = tx.Create(&binding).Error
			}
			if errors.Is(createError(err), ErrConflict) {
				return ErrAccountConflict
			}
			if err != nil {
				return err
			}
		}
		if err := revokeSessions(tx, user.ID); err != nil {
			return err
		}
		if err := tx.Model(&model.Employee{}).Where("id = ?", employeeID).Update("version", employee.Version+1).Error; err != nil {
			return err
		}
		result = model.WeComBindingView{EmployeeID: employeeID, Version: employee.Version + 1, MemberID: input.MemberID}
		event := model.AuditEvent{Type: auditWeComBinding, Action: action, Reason: reason, EmployeeID: &employeeID}
		return writeAudit(tx, actor, event, auditFields{"bound": bound, "version": employee.Version},
			auditFields{"bound": input.MemberID != nil, "version": result.Version})
	})
	return result, recordWriteError(err)
}

func (s *Service) cleanupWeCom(ctx context.Context, now time.Time) error {
	return s.db.WithContext(ctx).Exec("DELETE FROM oa_wecom_challenges WHERE expires_at <= ? ORDER BY expires_at LIMIT 500", now).Error
}
