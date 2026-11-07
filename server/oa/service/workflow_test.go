package service

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/flipped-aurora/gin-vue-admin/server/oa/model"
)

func TestWorkflowCanonicalSubmission(t *testing.T) {
	encode := func(title, form string) string {
		t.Helper()
		input, err := normalizeSubmission(model.SubmitInput{Type: "LEAVE", Title: title, Form: json.RawMessage(form)})
		if err != nil {
			t.Fatal(err)
		}
		body, err := json.Marshal(input)
		if err != nil {
			t.Fatal(err)
		}
		return digest(body)
	}
	first := encode("  演示申请  ", `{"reason":"请假 ","startDate":"2026-11-06","endDate":"2026-11-07"}`)
	second := encode("演示申请", `{ "endDate":"2026-11-07", "startDate":"2026-11-06", "reason":"请假" }`)
	changed := encode("演示申请", `{"reason":"请假","startDate":"2026-11-06","endDate":"2026-11-08"}`)
	if first != second || first == changed {
		t.Fatalf("canonical hashes do not reflect effective request contents")
	}
}

func TestWorkflowForms(t *testing.T) {
	for _, test := range []struct {
		name, kind, form string
		valid            bool
	}{
		{"leave leap day", "LEAVE", `{"startDate":"2024-02-29","endDate":"2024-02-29","reason":"休假"}`, true},
		{"leave order", "LEAVE", `{"startDate":"2026-11-07","endDate":"2026-11-06","reason":"休假"}`, false},
		{"leave invalid date", "LEAVE", `{"startDate":"2026-02-29","endDate":"2026-03-01","reason":"休假"}`, false},
		{"expense max", "EXPENSE", `{"expenseDate":"2026-11-06","amountCents":9007199254740991,"reason":"模拟费用"}`, true},
		{"expense unsafe int", "EXPENSE", `{"expenseDate":"2026-11-06","amountCents":9007199254740992,"reason":"模拟费用"}`, false},
		{"expense fraction", "EXPENSE", `{"expenseDate":"2026-11-06","amountCents":1.5,"reason":"模拟费用"}`, false},
		{"no employee field", "GENERAL", `{"content":"事项","employeeId":2}`, false},
		{"no route override", "GENERAL", `{"content":"事项","route":[]}`, false},
		{"no wrong form", "LEAVE", `{"content":"事项"}`, false},
		{"no blank text", "GENERAL", `{"content":" \n "}`, false},
		{"long Unicode form", "GENERAL", `{"content":"` + strings.Repeat("🚀", 2000) + `"}`, true},
		{"too long Unicode form", "GENERAL", `{"content":"` + strings.Repeat("🚀", 2001) + `"}`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := normalizeForm(test.kind, json.RawMessage(test.form))
			if (err == nil) != test.valid {
				t.Fatalf("unexpected validation result: %v", err)
			}
		})
	}
}

func TestWorkflowTaskVersionAndAssignment(t *testing.T) {
	step := 1
	request := model.ApprovalRequest{ApplicantID: 1, Status: model.RequestInReview, CurrentStep: &step}
	task := model.ApprovalTask{AssigneeID: 2, Status: model.TaskPending, Step: 1, Version: 1}
	if err := checkPendingTask(request, task, 2, 1); err != nil {
		t.Fatal(err)
	}
	task.AssigneeID, task.Version = 3, 2
	if err := checkPendingTask(request, task, 2, 1); !errors.Is(err, ErrForbidden) {
		t.Fatalf("old assignee should be forbidden: %v", err)
	}
	if err := checkPendingTask(request, task, 3, 1); !errors.Is(err, ErrStaleTask) {
		t.Fatalf("current assignee must present current version: %v", err)
	}
	if err := checkPendingTask(request, task, 3, 2); err != nil {
		t.Fatal(err)
	}
	task.Status = model.Approved
	if err := checkPendingTask(request, task, 3, 2); !errors.Is(err, ErrStaleTask) {
		t.Fatalf("resolved task should be stale: %v", err)
	}
}

func TestWorkflowSequentialRouteDoesNotSkipRepeatedAssignee(t *testing.T) {
	request := model.ApprovalRequest{ApplicantID: 10, Route: json.RawMessage(`[{"step":1,"assigneeId":20},{"step":2,"assigneeId":30}]`)}
	currentTask := model.ApprovalTask{AssigneeID: 30, Step: 1}
	next, err := nextRouteStep(request, currentTask.Step)
	if err != nil || next == nil || next.Step != 2 || next.AssigneeID != currentTask.AssigneeID {
		t.Fatalf("next step must survive transfer to its assignee: %v, %v", next, err)
	}
	if next, err := nextRouteStep(request, 2); err != nil || next != nil {
		t.Fatalf("last step should finish request: %v, %v", next, err)
	}
}

func TestWorkflowTaskActionInputAndIntentKey(t *testing.T) {
	target := uint(20)
	for _, test := range []struct {
		input model.TaskActionInput
		valid bool
	}{
		{model.TaskActionInput{Action: model.ActionApprove, Version: 1}, true},
		{model.TaskActionInput{Action: model.ActionApprove, Version: 1, TargetUserID: &target}, false},
		{model.TaskActionInput{Action: model.ActionApprove, Version: 0}, false},
		{model.TaskActionInput{Action: model.ActionReject, Version: 1, Comment: " "}, false},
		{model.TaskActionInput{Action: model.ActionReject, Version: 1, Comment: "理由"}, true},
		{model.TaskActionInput{Action: model.ActionTransfer, Version: 1, Comment: "理由", TargetUserID: &target}, true},
		{model.TaskActionInput{Action: model.ActionTransfer, Version: 1, Comment: "理由"}, false},
	} {
		_, err := normalizeTaskAction(test.input)
		if (err == nil) != test.valid {
			t.Fatalf("unexpected action validation for %+v: %v", test.input, err)
		}
	}
	for key, valid := range map[string]bool{
		"e7aa9b1e-c1d0-4f4d-8e7e-cf3d9cb8b119": true, "abcdefghijklmn_-": true,
		"too-short": false, "abcdefghijklmn_ ": false, "abcdefghijklmn_中": false, strings.Repeat("a", 129): false,
	} {
		if validIntentKey(key) != valid {
			t.Fatalf("incorrect key validation for %q", key)
		}
	}
}

func TestMockOutboxBackoffAndFixedMessages(t *testing.T) {
	for attempts, seconds := range []int{1, 2, 4, 8, 8} {
		if got := mockBackoff(uint(attempts)); got != time.Duration(seconds)*time.Second {
			t.Fatalf("attempt %d backoff %s", attempts, got)
		}
	}
	if mockTitle(model.TaskAssigned) != "有新的待办申请" ||
		mockTitle(model.RequestApproved) != "申请已通过" ||
		mockTitle(model.RequestRejected) != "申请已驳回" ||
		mockTitle("untrusted text") != "" {
		t.Fatal("mock notifications must use the fixed generic titles")
	}
}
