package utils

import (
	"context"
	"database/sql"
	"sync"

	"github.com/jinzhu/gorm"
	"github.com/pkg/errors"
	"go.uber.org/multierr"

	"github.com/smartcontractkit/chainlink/core/logger"
)

const (
	AdvisoryLockClassID_EthBroadcaster int32 = 0
	AdvisoryLockClassID_JobSpawner     int32 = 1
)

func GormTransaction(db *gorm.DB, fc func(tx *gorm.DB) error) (err error) {
	tx := db.Begin()
	defer func() {
		if r := recover(); r != nil {
			err = errors.Errorf("%+v", r)
			tx.Rollback()
		}
	}()

	err = fc(tx)

	if err == nil {
		err = errors.WithStack(tx.Commit().Error)
	}

	// Makesure rollback when Block error or Commit error
	if err != nil {
		tx.Rollback()
	}
	return
}

type PostgresAdvisoryLock struct {
	URI  string
	conn *sql.Conn
	db   *sql.DB
	mu   sync.Mutex
}

func (lock *PostgresAdvisoryLock) Close() error {
	var connErr, dbErr error

	if lock.conn != nil {
		connErr = lock.conn.Close()
		if connErr == sql.ErrConnDone {
			connErr = nil
		}
	}
	if lock.db != nil {
		dbErr = lock.db.Close()
		if dbErr == sql.ErrConnDone {
			dbErr = nil
		}
	}

	lock.db = nil
	lock.conn = nil

	return multierr.Combine(connErr, dbErr)
}

func (lock *PostgresAdvisoryLock) TryLock(ctx context.Context, classID int32, objectID int32) (err error) {
	lock.mu.Lock()
	defer lock.mu.Unlock()
	defer WrapIfError(&err, "TryAdvisoryLock failed")

	if lock.conn == nil {
		db, err := sql.Open("postgres", lock.URI)
		if err != nil {
			return err
		}
		lock.db = db

		// `database/sql`.DB does opaque connection pooling, but PG advisory locks are per-connection
		conn, err := db.Conn(ctx)
		if err != nil {
			lock.db.Close()
			lock.db = nil
			return err
		}
		lock.conn = conn
	}

	gotLock := false
	rows, err := lock.conn.QueryContext(ctx, "SELECT pg_try_advisory_lock($1, $2)", classID, objectID)
	if err != nil {
		return err
	}
	defer logger.ErrorIfCalling(rows.Close)
	gotRow := rows.Next()
	if !gotRow {
		return errors.New("query unexpectedly returned 0 rows")
	}
	if err := rows.Scan(&gotLock); err != nil {
		return err
	}
	if gotLock {
		return nil
	}
	return errors.Errorf("could not get advisory lock for classID, objectID %v, %v", classID, objectID)
}

func (lock *PostgresAdvisoryLock) Unlock(ctx context.Context, classID int32, objectID int32) error {
	lock.mu.Lock()
	defer lock.mu.Unlock()

	if lock.conn == nil {
		return nil
	}
	_, err := lock.conn.ExecContext(ctx, "SELECT pg_advisory_unlock($1, $2)", classID, objectID)
	return errors.Wrap(err, "AdvisoryUnlock failed")
}
