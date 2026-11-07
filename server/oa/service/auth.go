package service

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	system "github.com/flipped-aurora/gin-vue-admin/server/model/system"
	gva "github.com/flipped-aurora/gin-vue-admin/server/model/system/request"
	"github.com/flipped-aurora/gin-vue-admin/server/oa/model"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

const (
	tokenIssuer = "hr-oa-local-development"
	tokenTTL    = 30 * time.Minute
)

type oaClaims struct {
	gva.CustomClaims
	SessionEpoch uint64 `json:"oaSessionEpoch,omitempty"`
}

type loginAccount struct {
	system.SysUser
	SessionEpoch uint64
}

func identityQuery(db *gorm.DB) *gorm.DB {
	return db.Model(&system.SysUser{}).
		Joins("LEFT JOIN oa_account_sessions AS sessions ON sessions.user_id = sys_users.id").
		Where("NOT EXISTS (SELECT 1 FROM oa_employees AS e WHERE e.user_id = sys_users.id AND e.deleted_at IS NULL AND e.employment_status = ?)", model.EmploymentOffboarded)
}

func (s *Service) Login(ctx context.Context, input model.LoginInput) (model.LoginView, error) {
	username := strings.TrimSpace(input.Username)
	if username == "" || len(username) > 128 || len(input.Password) == 0 || len(input.Password) > 72 {
		return model.LoginView{}, ErrInvalid
	}

	var matches []loginAccount
	err := identityQuery(s.db.WithContext(ctx)).Select("sys_users.id, uuid, username, nick_name, password, authority_id, enable, COALESCE(sessions.epoch, 0) AS session_epoch").
		Where("username = ?", username).Limit(2).Find(&matches).Error
	if err != nil {
		return model.LoginView{}, err
	}
	var user loginAccount
	hash := s.dummyHash
	if len(matches) == 1 {
		user = matches[0]
		hash = []byte(user.Password)
	}
	if bcrypt.CompareHashAndPassword(hash, []byte(input.Password)) != nil || user.Enable != 1 || user.UUID == uuid.Nil {
		return model.LoginView{}, ErrUnauthorized
	}

	_, session, err := s.session(ctx, user.SysUser)
	if err != nil {
		return model.LoginView{}, err
	}
	now := time.Now().UTC()
	expiry := jwt.NewNumericDate(now.Add(tokenTTL))
	claims := oaClaims{SessionEpoch: user.SessionEpoch, CustomClaims: gva.CustomClaims{
		BaseClaims: gva.BaseClaims{
			UUID: user.UUID, ID: user.ID, Username: user.Username,
			NickName: user.NickName, AuthorityId: user.AuthorityId,
		},
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer: tokenIssuer, Audience: jwt.ClaimStrings{"GVA"}, Subject: strconv.FormatUint(uint64(user.ID), 10),
			IssuedAt: jwt.NewNumericDate(now), NotBefore: jwt.NewNumericDate(now), ExpiresAt: expiry,
		},
	}}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.signingKey)
	return model.LoginView{SessionView: session, Token: token, ExpiresAt: expiry.Time.UTC()}, err
}

func (s *Service) Authenticate(ctx context.Context, raw string) (model.Actor, model.SessionView, error) {
	if raw == "" || len(raw) > 4096 {
		return model.Actor{}, model.SessionView{}, ErrUnauthorized
	}
	var claims oaClaims
	token, err := jwt.ParseWithClaims(raw, &claims, func(*jwt.Token) (any, error) { return s.signingKey, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithIssuer(tokenIssuer),
		jwt.WithAudience("GVA"), jwt.WithExpirationRequired(), jwt.WithIssuedAt())
	userID := claims.BaseClaims.ID
	if err != nil || !token.Valid || userID == 0 || claims.UUID == uuid.Nil ||
		claims.Subject != strconv.FormatUint(uint64(userID), 10) {
		return model.Actor{}, model.SessionView{}, ErrUnauthorized
	}

	var user loginAccount
	err = identityQuery(s.db.WithContext(ctx)).Select("sys_users.id, uuid, username, nick_name, authority_id, enable, COALESCE(sessions.epoch, 0) AS session_epoch").
		Take(&user, "sys_users.id = ? AND uuid = ? AND enable = ? AND COALESCE(sessions.epoch, 0) = ?", userID, claims.UUID, 1, claims.SessionEpoch).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = ErrUnauthorized
	}
	if err != nil {
		return model.Actor{}, model.SessionView{}, err
	}
	actor, session, err := s.session(ctx, user.SysUser)
	actor.SessionEpoch = claims.SessionEpoch
	return actor, session, err
}

func (s *Service) session(ctx context.Context, user system.SysUser) (model.Actor, model.SessionView, error) {
	role := model.RoleName(user.AuthorityId)
	if role == "" {
		return model.Actor{}, model.SessionView{}, ErrForbidden
	}
	db := s.db.WithContext(ctx)
	var authority system.SysAuthority
	err := db.Select("authority_id").Take(&authority, "authority_id = ? AND deleted_at IS NULL", user.AuthorityId).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = ErrForbidden
	}
	if err != nil {
		return model.Actor{}, model.SessionView{}, err
	}

	var employee model.Employee
	err = db.Select("id").Take(&employee, "user_id = ?", user.ID).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return model.Actor{}, model.SessionView{}, err
	}
	actor := model.Actor{UserID: user.ID, AuthorityID: user.AuthorityId}
	if err == nil {
		actor.EmployeeID = &employee.ID
	}
	return actor, model.SessionView{
		User: model.UserView{
			ID: user.ID, Username: user.Username, DisplayName: user.NickName,
			Role: role, EmployeeID: actor.EmployeeID,
		},
		Permissions: model.Permissions{
			CanWriteEmployees: actor.CanWrite(), CanReadSalary: actor.CanReadSalary(),
			CanSubmitRequests: actor.CanSubmitRequests(), CanApproveRequests: actor.CanApproveRequests(),
			CanManageDepartments: actor.CanWrite(), CanManageAnnualLeave: actor.CanWrite(),
			CanImportEmployees: actor.CanWrite(), CanExportEmployees: true,
		},
	}, nil
}
