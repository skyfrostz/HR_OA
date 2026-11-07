package service

import (
	"context"
	"crypto/sha256"
	"database/sql"
	sqldriver "database/sql/driver"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/flipped-aurora/gin-vue-admin/server/oa/model"
	driver "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
)

const intentDeadline = 5 * time.Second

var (
	ErrIntentReused     = errors.New("idempotency key reused")
	ErrIntentInProgress = errors.New("idempotency intent in progress")
	ErrStaleTask        = errors.New("stale task")
	ErrRouteUnavailable = errors.New("approval route unavailable")
	ErrTransferLimit    = errors.New("transfer limit reached")
)

type IntentResult struct {
	Status   int
	Body     json.RawMessage
	Replayed bool
}

func validIntentKey(key string) bool {
	if len(key) < 16 || len(key) > 128 {
		return false
	}
	for _, c := range []byte(key) {
		if !(c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '_' || c == '-') {
			return false
		}
	}
	return true
}

func digest(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}

func (s *Service) withIntent(ctx context.Context, actorID uint, route, key string, normalized any, status int,
	authorize func(*gorm.DB) error, apply func(*gorm.DB) (any, error),
) (IntentResult, error) {
	if !validIntentKey(key) {
		return IntentResult{}, ErrInvalid
	}
	body, err := json.Marshal(normalized)
	if err != nil {
		return IntentResult{}, err
	}
	intent := model.WorkflowIntent{ActorID: actorID, Route: route, IntentKey: key, RequestHash: digest(body)}
	lockKey := digest([]byte(strconv.FormatUint(uint64(actorID), 10) + "\x00" + route + "\x00" + key))

	ctx, cancel := context.WithTimeout(ctx, intentDeadline)
	defer cancel()
	var result IntentResult
	err = s.db.WithContext(ctx).Connection(func(connection *gorm.DB) error {
		conn := connection.Statement.ConnPool.(*sql.Conn)
		var acquired sql.NullInt64
		// The advisory lock gives a bounded wait for concurrent duplicate intents.
		// The transactional unique index still owns the durable claim.
		if err := conn.QueryRowContext(ctx, "SELECT GET_LOCK(SHA2(CONCAT(DATABASE(), ':', ?), 256), 1)", lockKey).
			Scan(&acquired); err != nil || !acquired.Valid {
			discardConnection(conn)
			if err != nil {
				return err
			}
			return ErrIntentInProgress
		}
		if acquired.Int64 != 1 {
			return ErrIntentInProgress
		}
		defer releaseIntentLock(conn, lockKey)

		return connection.Transaction(func(tx *gorm.DB) error {
			tx = s.notificationTx(tx)
			if err := authorize(tx); err != nil {
				return err
			}
			var previous model.WorkflowIntent
			err := tx.Take(&previous, "actor_id = ? AND route = ? AND intent_key = ?", actorID, route, key).Error
			switch {
			case err == nil:
				if previous.RequestHash != intent.RequestHash {
					return ErrIntentReused
				}
				if previous.ResultStatus == 0 || len(previous.Result) == 0 {
					return ErrIntentInProgress
				}
				result = IntentResult{Status: previous.ResultStatus, Body: previous.Result, Replayed: true}
				return nil
			case !errors.Is(err, gorm.ErrRecordNotFound):
				return err
			}
			if err := tx.Create(&intent).Error; err != nil {
				if errors.Is(createError(err), ErrConflict) {
					return ErrIntentInProgress
				}
				return err
			}
			detail, err := apply(tx)
			if err != nil {
				return err
			}
			snapshot, err := json.Marshal(detail)
			if err != nil {
				return err
			}
			err = tx.Model(&intent).Updates(map[string]any{"result_status": status, "result": snapshot}).Error
			if err != nil {
				return err
			}
			result = IntentResult{Status: status, Body: snapshot}
			return nil
		}, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	})
	if err != nil {
		var mysqlError *driver.MySQLError
		if errors.Is(ctx.Err(), context.DeadlineExceeded) ||
			(errors.As(err, &mysqlError) && (mysqlError.Number == 1205 || mysqlError.Number == 1213)) {
			err = ErrIntentInProgress
		}
		return IntentResult{}, err
	}
	return result, nil
}

func releaseIntentLock(conn *sql.Conn, key string) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	var released sql.NullInt64
	err := conn.QueryRowContext(ctx, "SELECT RELEASE_LOCK(SHA2(CONCAT(DATABASE(), ':', ?), 256))", key).Scan(&released)
	if err != nil || !released.Valid || released.Int64 != 1 {
		discardConnection(conn)
	}
}

func discardConnection(conn *sql.Conn) {
	// A failed RELEASE_LOCK must not return a session with a live lock to the pool.
	_ = conn.Raw(func(any) error { return sqldriver.ErrBadConn })
}
