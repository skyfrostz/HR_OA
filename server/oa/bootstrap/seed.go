package bootstrap

import (
	"errors"
	"time"

	system "github.com/flipped-aurora/gin-vue-admin/server/model/system"
	"github.com/flipped-aurora/gin-vue-admin/server/oa/model"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Match GVA's join table and preserve its composite key without migrating
// unrelated menus, permissions or upstream application tables.
type userAuthority struct {
	SysUserID      uint `gorm:"column:sys_user_id;primaryKey;autoIncrement:false"`
	SysAuthorityID uint `gorm:"column:sys_authority_authority_id;primaryKey;autoIncrement:false"`
}

func (userAuthority) TableName() string { return "sys_user_authority" }

func migrateSchema(db *gorm.DB) error {
	identity := db.Session(&gorm.Session{NewDB: true})
	identity.Config.IgnoreRelationshipsWhenMigrating = true
	if err := identity.AutoMigrate(&system.SysAuthority{}, &system.SysUser{}, &userAuthority{}); err != nil {
		return err
	}
	if err := db.AutoMigrate(
		&model.Department{}, &model.Employee{}, &model.Contract{}, &model.ContractFile{},
		&model.AnnualLeave{}, &model.EmployeeImport{},
		&model.ApprovalRequest{}, &model.ApprovalTask{}, &model.ApprovalAction{},
		&model.WorkflowIntent{}, &model.NotificationEvent{}, &model.MockNotification{},
		&model.AuditEvent{}, &model.AccountSession{}, &model.WorkflowGuard{}, &model.ApprovalHandoff{},
		&model.LeaveType{}, &model.LeaveCalendar{}, &model.LeaveLedger{}, &model.LeaveSlot{}, &model.ApprovalRule{},
	); err != nil {
		return err
	}
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&model.WorkflowGuard{ID: 1}).Error; err != nil {
			return err
		}
		var guard model.WorkflowGuard
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Take(&guard, 1).Error; err != nil {
			return err
		}
		var calendar model.LeaveCalendar
		if err := tx.Where("id = ?", 1).Attrs(model.LeaveCalendar{ID: 1, Version: 1, WorkingWeekdays: []int{1, 2, 3, 4, 5}, Exceptions: []model.CalendarException{}}).FirstOrCreate(&calendar).Error; err != nil {
			return err
		}
		for _, kind := range []model.LeaveType{
			{ID: 1, Name: "年假", UsesBalance: true, Enabled: true, Version: 1},
			{ID: 2, Name: "事假", Enabled: true, Version: 1},
		} {
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&kind).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func seed(db *gorm.DB, password string) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := ensureRoles(tx); err != nil {
			return err
		}

		for _, account := range []struct {
			username, displayName, employeeNo string
			authorityID                       uint
		}{
			{"hr_admin", "演示人事管理员", "", model.HRAdmin},
			{"hr_viewer", "演示人事查看员", "", model.HRViewer},
			{"employee", "演示员工甲", "OA-DEMO-001", model.EmployeeRole},
			{"manager", "演示部门负责人", "OA-DEMO-003", model.ApproverRole},
			{"finance", "演示复核员", "OA-DEMO-004", model.ApproverRole},
			{"delegate", "演示转交审批人", "OA-DEMO-005", model.ApproverRole},
		} {
			id, err := seedAccount(tx, account.username, account.displayName, account.authorityID, password)
			if err != nil {
				return err
			}
			if account.employeeNo != "" {
				if err := seedEmployee(tx, account.employeeNo, account.displayName, &id); err != nil {
					return err
				}
			}
			if account.authorityID == model.EmployeeRole {
				if err := seedEmployee(tx, "OA-DEMO-002", "演示员工乙", nil); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func seedAccount(tx *gorm.DB, username, displayName string, authorityID uint, password string) (uint, error) {
	id := uuid.NewSHA1(uuid.NameSpaceOID, []byte("hr-oa-local-development:"+username))
	var user system.SysUser
	err := tx.Unscoped().Take(&user, "username = ?", username).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return 0, err
		}
		user = system.SysUser{UUID: id, Username: username, NickName: displayName, Password: string(hash), AuthorityId: authorityID, Enable: 1}
		if err := tx.Create(&user).Error; err != nil {
			return 0, err
		}
		membership := userAuthority{SysUserID: user.ID, SysAuthorityID: authorityID}
		if err := tx.Create(&membership).Error; err != nil {
			return 0, err
		}
	} else if err != nil {
		return 0, err
	} else if user.UUID != id {
		return 0, errors.New("development username conflicts with existing identity")
	}
	return user.ID, nil
}

func seedEmployee(tx *gorm.DB, number, name string, userID *uint) error {
	if userID != nil {
		var bound model.Employee
		err := tx.Unscoped().Select("id").Take(&bound, "user_id = ?", *userID).Error
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
	}

	var employee model.Employee
	err := tx.Unscoped().Select("id").Take(&employee, "employee_no = ?", number).Error
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		if err == nil && userID != nil {
			return errors.New("development employee number conflicts with existing employee")
		}
		return err
	}
	return tx.Create(&model.Employee{
		UserID: userID, EmployeeNo: number, Name: name, Department: "演示部门", SalaryCents: 600000,
		BirthDate: time.Date(1995, 5, 1, 0, 0, 0, 0, time.UTC),
		HireDate:  time.Date(2023, 6, 1, 0, 0, 0, 0, time.UTC),
	}).Error
}

func ensureRoles(tx *gorm.DB) error {
	for _, id := range []uint{model.HRAdmin, model.HRViewer, model.EmployeeRole, model.ApproverRole} {
		role := system.SysAuthority{AuthorityId: id, AuthorityName: model.RoleName(id)}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&role).Error; err != nil {
			return err
		}
		var persisted system.SysAuthority
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Take(&persisted, "authority_id = ?", id).Error
		if err != nil {
			return err
		}
		if persisted.DeletedAt != nil || persisted.AuthorityName != role.AuthorityName {
			return errors.New("development authority conflicts with existing identity")
		}
	}
	return nil
}

func initializeAdministrator(db *gorm.DB, username, password string, production bool) error {
	if username == "" && !production {
		return nil
	}
	return db.Transaction(func(tx *gorm.DB) error {
		var guard model.WorkflowGuard
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Take(&guard, 1).Error; err != nil {
			return err
		}
		if production {
			var identifiers []uuid.UUID
			for _, name := range []string{"hr_admin", "hr_viewer", "employee", "manager", "finance", "delegate"} {
				identifiers = append(identifiers, uuid.NewSHA1(uuid.NameSpaceOID, []byte("hr-oa-local-development:"+name)))
			}
			var enabledSeed int64
			if err := tx.Model(&system.SysUser{}).Where("uuid IN ? AND enable = ?", identifiers, 1).Count(&enabledSeed).Error; err != nil {
				return err
			}
			if enabledSeed != 0 {
				return errors.New("production cannot use enabled development identities")
			}
		}
		var users int64
		if err := tx.Unscoped().Model(&system.SysUser{}).Count(&users).Error; err != nil || users != 0 {
			return err
		}
		if username == "" {
			return errors.New("initial administrator configuration is required for an empty production identity store")
		}
		if err := ensureRoles(tx); err != nil {
			return err
		}
		identifier, err := uuid.NewRandom()
		if err != nil {
			return err
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		user := system.SysUser{UUID: identifier, Username: username, NickName: "人事管理员",
			Password: string(hash), AuthorityId: model.HRAdmin, Enable: 1}
		if err := tx.Create(&user).Error; err != nil {
			return err
		}
		return tx.Create(&userAuthority{SysUserID: user.ID, SysAuthorityID: model.HRAdmin}).Error
	})
}
