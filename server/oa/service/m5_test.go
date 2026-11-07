package service

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/flipped-aurora/gin-vue-admin/server/oa/model"
)

func TestLeaveHalfDaysAcrossYearsWithCalendarExceptions(t *testing.T) {
	period := model.LeavePeriod{LeaveTypeID: 1, StartDate: "2026-12-31", StartHalf: "PM", EndDate: "2027-01-04", EndHalf: "AM"}
	cal := model.LeaveCalendar{Version: 9, WorkingWeekdays: []int{1, 2, 3, 4, 5}, Exceptions: []model.CalendarException{
		{Date: "2027-01-01", Working: false}, {Date: "2027-01-02", Working: true},
	}}
	result, err := calculateLeave(period, model.LeaveType{ID: 1, Name: "年假", Version: 2, UsesBalance: true}, cal)
	if err != nil {
		t.Fatal(err)
	}
	wantYears := []model.LeaveYear{{Year: 2026, HalfDays: 1}, {Year: 2027, HalfDays: 3}}
	wantDays := []model.LeaveDay{
		{Date: "2026-12-31", Halves: []string{"PM"}},
		{Date: "2027-01-02", Halves: []string{"AM", "PM"}},
		{Date: "2027-01-04", Halves: []string{"AM"}},
	}
	if !reflect.DeepEqual(result.Years, wantYears) || !reflect.DeepEqual(result.Days, wantDays) ||
		result.TotalHalfDays != 4 || result.CalendarVersion != 9 || result.LeaveTypeVersion != 2 {
		t.Fatalf("unexpected calculation: %+v", result)
	}
}

func TestLeaveRejectsReversedHalfDayAndNonworkingInterval(t *testing.T) {
	cal := model.LeaveCalendar{WorkingWeekdays: []int{1, 2, 3, 4, 5}}
	for _, period := range []model.LeavePeriod{
		{StartDate: "2026-11-06", EndDate: "2026-11-06", StartHalf: "PM", EndHalf: "AM"},
		{StartDate: "2026-11-08", EndDate: "2026-11-08", StartHalf: "AM", EndHalf: "PM"},
		{StartDate: "2026-01-01", EndDate: "2027-01-02", StartHalf: "AM", EndHalf: "PM"},
	} {
		if _, err := calculateLeave(period, model.LeaveType{}, cal); !errors.Is(err, ErrInvalid) {
			t.Fatalf("invalid leave accepted: %+v, %v", period, err)
		}
	}
}

func TestLegacyLeaveHashInputRemainsCanonicalButCannotCreateNewRequest(t *testing.T) {
	old := json.RawMessage(`{"startDate":"2026-11-06","endDate":"2026-11-06","reason":"旧申请"}`)
	normalized, err := normalizeForm("LEAVE", old)
	if err != nil || string(normalized) != string(old) {
		t.Fatalf("M4 canonical form changed: %s, %v", normalized, err)
	}
	var form leaveForm
	if err := json.Unmarshal(old, &form); err != nil {
		t.Fatal(err)
	}
	if _, err := prepareLeaveSubmission(nil, 1, form); !errors.Is(err, ErrInvalid) {
		t.Fatalf("new unversioned leave must fail, got %v", err)
	}
}

func TestRulesOverlapAtInclusiveAmountBoundary(t *testing.T) {
	departmentA, departmentB := uint(1), uint(2)
	firstMax, secondMin := int64(1000), int64(1000)
	first := model.ApprovalRule{DepartmentID: &departmentA, MaxAmountCents: &firstMax}
	second := model.ApprovalRule{MinAmountCents: &secondMin}
	if !rulesOverlap(first, second) {
		t.Fatal("a global rule and a scoped rule overlap at the inclusive boundary")
	}
	second.DepartmentID = &departmentB
	if rulesOverlap(first, second) {
		t.Fatal("different direct departments do not overlap")
	}
	second.DepartmentID = &departmentA
	secondMin++
	if rulesOverlap(first, second) {
		t.Fatal("disjoint amount ranges must coexist")
	}
}

func TestExceptionResolvesOverridesAndRejectsFutureSelfApproval(t *testing.T) {
	route, err := json.Marshal([]model.RouteStep{
		{Step: 1, AssigneeID: 2, Name: "负责人"},
		{Step: 2, AssigneeID: 3, Name: "复核"},
		{Step: 3, AssigneeID: 4, Name: "终审"},
	})
	if err != nil {
		t.Fatal(err)
	}
	request := model.ApprovalRequest{ID: 1, ApplicantID: 8, Route: route}
	task := model.ApprovalTask{ID: 12, Step: 1, Version: 5, AssigneeID: 2}
	available := map[uint]bool{2: true, 8: true, 9: true}
	handoffs := handoffMap{3: 8}
	exception, err := exceptionFor(request, task, available, handoffs)
	if err != nil || exception == nil || exception.Step != 2 || exception.TaskID != 12 || exception.Version != 5 {
		t.Fatalf("unexpected first exception %+v, %v", exception, err)
	}
	request.RouteOverrides = map[int]model.RouteStep{2: {Step: 2, AssigneeID: 9, Name: "复核"}}
	exception, err = exceptionFor(request, task, available, handoffs)
	if err != nil || exception == nil || exception.Step != 3 {
		t.Fatalf("future override did not advance to next exception: %+v, %v", exception, err)
	}
}
