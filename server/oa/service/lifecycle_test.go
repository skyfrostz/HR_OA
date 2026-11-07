package service

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/flipped-aurora/gin-vue-admin/server/oa/model"
)

func TestFutureHandoffResolvesChainAndPreventsSelfApproval(t *testing.T) {
	steps, err := json.Marshal([]model.RouteStep{
		{Step: 1, AssigneeID: 11},
		{Step: 2, AssigneeID: 12},
	})
	if err != nil {
		t.Fatal(err)
	}
	current := 1
	req := model.ApprovalRequest{ApplicantID: 7, CurrentStep: &current, Route: steps}
	handoffs := handoffMap{11: 17, 12: 18, 18: 19}
	target := uint(7)
	if _, err := futureHandoff(req, handoffs, 19, &target); !errors.Is(err, ErrHandoffConflict) {
		t.Fatalf("handoff to the applicant must fail, got %v", err)
	}
	target = 20
	if affected, err := futureHandoff(req, handoffs, 19, &target); err != nil || !affected {
		t.Fatalf("future node should follow the chain, got %t, %v", affected, err)
	}
	if affected, err := futureHandoff(req, handoffs, 17, &target); err != nil || affected {
		t.Fatalf("current step is not a future node, got %t, %v", affected, err)
	}
	handoffs[19] = 12
	if _, err := handoffs.resolve(12); !errors.Is(err, ErrHandoffConflict) {
		t.Fatalf("handoff cycle should fail, got %v", err)
	}
}

func TestAccountActionRejectsPrivilegeAndPasswordInReason(t *testing.T) {
	username := "fixture-user"
	password := "fixture-password-only"
	role := "HR_ADMIN"
	input := model.AccountAction{
		Action: "CREATE", Version: 1, Reason: "fixture", Username: &username, Password: &password, Role: &role,
	}
	if _, _, err := validateAccountAction(input); !errors.Is(err, ErrInvalid) {
		t.Fatalf("privileged role was accepted, got %v", err)
	}
	role = "EMPLOYEE"
	input.Reason += " " + password
	if _, _, err := validateAccountAction(input); !errors.Is(err, ErrInvalid) {
		t.Fatalf("password in the reason was accepted, got %v", err)
	}
	input.Reason = "开通员工账号"
	if _, authority, err := validateAccountAction(input); err != nil || authority != model.EmployeeRole {
		t.Fatalf("employee credentials rejected, role=%d, err=%v", authority, err)
	}
}

func TestContractTransitionsRequireCorrectPriorStateAndDate(t *testing.T) {
	start := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	sign := start.AddDate(0, 0, 2)
	draft := model.Contract{Status: "DRAFT", StartDate: start, Version: 3}
	if _, err := contractTransition(draft, "TERMINATE", sign); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("draft termination should fail, got %v", err)
	}
	updates, err := contractTransition(draft, "SIGN", sign)
	if err != nil || updates["status"] != "SIGNED" || updates["signed_date"] != sign {
		t.Fatalf("sign should record the date, got %v, %v", updates, err)
	}
	signed := draft
	signed.Status, signed.SignedDate = "SIGNED", &sign
	if _, err := contractTransition(signed, "TERMINATE", start); !errors.Is(err, ErrInvalid) {
		t.Fatalf("termination before signing should fail, got %v", err)
	}
	if _, err := contractTransition(signed, "SIGN", sign); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("second signature should fail, got %v", err)
	}
}

func TestWorkflowDateFilterUsesShanghaiMidnight(t *testing.T) {
	date, err := businessMidnight("2026-11-07")
	if err != nil || date.UTC().Format(time.RFC3339) != "2026-11-06T16:00:00Z" {
		t.Fatalf("wrong UTC date boundary, got %s, %v", date.UTC(), err)
	}
	if _, err := businessMidnight("2026-02-30"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid calendar date accepted, got %v", err)
	}
}
