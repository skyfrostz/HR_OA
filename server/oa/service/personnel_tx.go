package service

import (
	"errors"

	driver "github.com/go-sql-driver/mysql"
)

func recordWriteError(err error) error {
	var mysqlError *driver.MySQLError
	if errors.As(err, &mysqlError) && (mysqlError.Number == 1205 || mysqlError.Number == 1213) {
		return ErrStaleRecord
	}
	return err
}
