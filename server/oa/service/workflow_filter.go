package service

import (
	"time"

	"github.com/flipped-aurora/gin-vue-admin/server/oa/model"
	"gorm.io/gorm"
)

func applyWorkflowFilter(db *gorm.DB, input model.WorkflowFilter, requestPrefix, timeColumn string) (*gorm.DB, error) {
	switch input.Type {
	case "":
	case "LEAVE", "EXPENSE", "GENERAL":
		db = db.Where(requestPrefix+"type = ?", input.Type)
	default:
		return nil, ErrInvalid
	}
	switch input.Status {
	case "":
	case model.RequestInReview, model.Approved, model.Rejected:
		db = db.Where(requestPrefix+"status = ?", input.Status)
	default:
		return nil, ErrInvalid
	}
	if input.FromDate != "" && input.ToDate != "" && input.FromDate > input.ToDate {
		return nil, ErrInvalid
	}
	if input.FromDate != "" {
		from, err := businessMidnight(input.FromDate)
		if err != nil {
			return nil, err
		}
		db = db.Where(timeColumn+" >= ?", from.UTC())
	}
	if input.ToDate != "" {
		to, err := businessMidnight(input.ToDate)
		if err != nil {
			return nil, err
		}
		db = db.Where(timeColumn+" <= ?", to.AddDate(0, 0, 1).Add(-time.Microsecond).UTC())
	}
	return db, nil
}

func businessMidnight(value string) (time.Time, error) {
	date, err := parseDate(value)
	if err != nil {
		return time.Time{}, err
	}
	return time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, businessTime), nil
}
