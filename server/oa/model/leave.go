package model

import "time"

type LeaveType struct {
	ID          uint   `gorm:"primaryKey" json:"id"`
	Name        string `gorm:"size:60;not null;uniqueIndex" json:"name"`
	UsesBalance bool   `gorm:"not null" json:"usesBalance"`
	Enabled     bool   `gorm:"not null" json:"enabled"`
	Version     int    `gorm:"not null" json:"version"`
}

func (LeaveType) TableName() string { return "oa_leave_types" }

type LeaveTypeInput struct {
	Name        string `json:"name"`
	UsesBalance *bool  `json:"usesBalance"`
	Enabled     *bool  `json:"enabled"`
	Version     int    `json:"version"`
}
type LeaveTypes struct {
	List []LeaveType `json:"list"`
}

type LeaveCalendar struct {
	ID              uint                `gorm:"primaryKey;autoIncrement:false" json:"-"`
	Version         int                 `gorm:"not null" json:"version"`
	WorkingWeekdays []int               `gorm:"serializer:json;type:json;not null" json:"workingWeekdays"`
	Exceptions      []CalendarException `gorm:"serializer:json;type:json;not null" json:"exceptions"`
}

func (LeaveCalendar) TableName() string { return "oa_leave_calendar" }

type CalendarException struct {
	Date    string `json:"date"`
	Working bool   `json:"working"`
}
type CalendarInput struct {
	Version         int                      `json:"version"`
	WorkingWeekdays []int                    `json:"workingWeekdays"`
	Exceptions      []CalendarExceptionInput `json:"exceptions"`
}
type CalendarExceptionInput struct {
	Date    string `json:"date"`
	Working *bool  `json:"working"`
}

type LeavePeriod struct {
	LeaveTypeID uint   `json:"leaveTypeId,omitempty"`
	StartDate   string `json:"startDate"`
	EndDate     string `json:"endDate"`
	StartHalf   string `json:"startHalf,omitempty"`
	EndHalf     string `json:"endHalf,omitempty"`
}

type LeaveCalculation struct {
	LeavePeriod
	LeaveTypeName    string      `json:"leaveTypeName"`
	LeaveTypeVersion int         `json:"leaveTypeVersion"`
	CalendarVersion  int         `json:"calendarVersion"`
	UsesBalance      bool        `json:"usesBalance"`
	TotalHalfDays    int         `json:"totalHalfDays"`
	Years            []LeaveYear `json:"years"`
	Days             []LeaveDay  `json:"days"`
}
type LeaveYear struct {
	Year     int `json:"year"`
	HalfDays int `json:"halfDays"`
}
type LeaveDay struct {
	Date   string   `json:"date"`
	Halves []string `json:"halves"`
}
type LeavePreview struct {
	Calculation LeaveCalculation  `json:"calculation"`
	Balances    []AnnualLeaveView `json:"balances"`
	CanSubmit   bool              `json:"canSubmit"`
	Issues      []string          `json:"issues"`
}

const (
	LeaveReserve = "RESERVE"
	LeaveConsume = "CONSUME"
	LeaveRelease = "RELEASE"
	LeaveReverse = "REVERSE"
)

// LeaveLedger entries are append-only. Employee and request row locks govern transitions.
type LeaveLedger struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	EmployeeID uint      `gorm:"not null;index:ix_oa_leave_employee_year,priority:1" json:"-"`
	RequestID  uint      `gorm:"not null;uniqueIndex:ux_oa_leave_intent,priority:1" json:"requestId"`
	Year       int       `gorm:"not null;uniqueIndex:ux_oa_leave_intent,priority:2;index:ix_oa_leave_employee_year,priority:2" json:"year"`
	Kind       string    `gorm:"size:12;not null;uniqueIndex:ux_oa_leave_intent,priority:3" json:"kind"`
	HalfDays   int       `gorm:"not null" json:"halfDays"`
	CreatedAt  time.Time `json:"createdAt"`
}

func (LeaveLedger) TableName() string { return "oa_leave_ledger" }

// LeaveSlot is an active occupancy index. Immutable historical dates remain on the request.
type LeaveSlot struct {
	EmployeeID uint      `gorm:"primaryKey;autoIncrement:false"`
	Date       time.Time `gorm:"primaryKey;type:date"`
	Half       string    `gorm:"primaryKey;size:2"`
	RequestID  uint      `gorm:"not null;index"`
}

func (LeaveSlot) TableName() string { return "oa_leave_slots" }
