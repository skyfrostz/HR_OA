package bootstrap

import (
	"context"
	"database/sql"
	sqldriver "database/sql/driver"
	"errors"
	"time"

	"github.com/flipped-aurora/gin-vue-admin/server/oa/model"
	"gorm.io/gorm"
)

const schemaVersion = 6

type schemaMigration struct {
	Version   int `gorm:"primaryKey;autoIncrement:false"`
	AppliedAt time.Time
}

func (schemaMigration) TableName() string { return "oa_schema_migrations" }

// Existing M1–M5 installations predate the ledger. The M6 reconciliation
// applies additive changes and records success after DDL and workflow defaults.
func migrate(db *gorm.DB) error {
	pool, err := db.DB()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	conn, err := pool.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	// MySQL holds the advisory lock on this connection. GORM uses its usual
	// pool and fresh statements to perform the migration.
	const lockName = "SHA2(CONCAT('hr-oa-schema:', DATABASE()), 256)"
	var locked sql.NullInt64
	if err := conn.QueryRowContext(ctx, "SELECT GET_LOCK("+lockName+", 10)").Scan(&locked); err != nil {
		discardMigrationConnection(conn)
		return errors.New("OA schema lock failed")
	}
	if !locked.Valid || locked.Int64 != 1 {
		return errors.New("OA schema lock unavailable")
	}
	defer func() {
		release, stop := context.WithTimeout(context.Background(), 2*time.Second)
		defer stop()
		var unlocked sql.NullInt64
		if err := conn.QueryRowContext(release, "SELECT RELEASE_LOCK("+lockName+")").Scan(&unlocked); err != nil ||
			!unlocked.Valid || unlocked.Int64 != 1 {
			discardMigrationConnection(conn)
		}
	}()
	if err := db.AutoMigrate(&schemaMigration{}); err != nil {
		return err
	}
	var current schemaMigration
	err = db.Order("version DESC").Take(&current).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if current.Version > schemaVersion {
		return errors.New("OA schema is newer than this binary")
	}
	if current.Version == schemaVersion {
		return nil
	}
	if err := migrateSchema(db); err != nil {
		return err
	}
	if err := db.AutoMigrate(&model.WeComBinding{}, &model.WeComChallenge{}); err != nil {
		return err
	}
	// AutoMigrate does not consistently relax a historical NOT NULL
	// constraint on indexed columns across MySQL versions.
	if err := db.Exec("ALTER TABLE oa_notification_outbox MODIFY action_id BIGINT UNSIGNED NULL").Error; err != nil {
		return err
	}
	return db.Create(&schemaMigration{Version: schemaVersion, AppliedAt: time.Now().UTC()}).Error
}

func discardMigrationConnection(conn *sql.Conn) {
	_ = conn.Raw(func(any) error { return sqldriver.ErrBadConn })
}
