package service

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/flipped-aurora/gin-vue-admin/server/oa/model"
)

type leaveForm struct {
	StartDate string `json:"startDate"`
	EndDate   string `json:"endDate"`
	Reason    string `json:"reason"`
}

type expenseForm struct {
	ExpenseDate string `json:"expenseDate"`
	AmountCents int64  `json:"amountCents"`
	Reason      string `json:"reason"`
}

type generalForm struct {
	Content string `json:"content"`
}

func normalizeSubmission(input model.SubmitInput) (model.SubmitInput, error) {
	input.Title = strings.TrimSpace(input.Title)
	if !workflowText(input.Title, 1, 100) || (input.PreviousRequestID != nil && *input.PreviousRequestID == 0) {
		return input, ErrInvalid
	}
	form, err := normalizeForm(input.Type, input.Form)
	input.Form = form
	return input, err
}

func normalizeForm(kind string, raw json.RawMessage) (json.RawMessage, error) {
	switch kind {
	case "LEAVE":
		var form leaveForm
		if !decodeForm(raw, &form) {
			return nil, ErrInvalid
		}
		form.Reason = strings.TrimSpace(form.Reason)
		start, startErr := parseDate(form.StartDate)
		end, endErr := parseDate(form.EndDate)
		if startErr != nil || endErr != nil || start.After(end) || !workflowText(form.Reason, 1, 1000) {
			return nil, ErrInvalid
		}
		return json.Marshal(form)
	case "EXPENSE":
		var form expenseForm
		if !decodeForm(raw, &form) {
			return nil, ErrInvalid
		}
		form.Reason = strings.TrimSpace(form.Reason)
		if _, err := parseDate(form.ExpenseDate); err != nil || form.AmountCents < 1 || form.AmountCents > maxSalaryCents ||
			!workflowText(form.Reason, 1, 1000) {
			return nil, ErrInvalid
		}
		return json.Marshal(form)
	case "GENERAL":
		var form generalForm
		if !decodeForm(raw, &form) {
			return nil, ErrInvalid
		}
		form.Content = strings.TrimSpace(form.Content)
		if !workflowText(form.Content, 1, 2000) {
			return nil, ErrInvalid
		}
		return json.Marshal(form)
	default:
		return nil, ErrInvalid
	}
}

func decodeForm(raw json.RawMessage, destination any) bool {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	return decoder.Decode(destination) == nil && decoder.Decode(new(any)) == io.EOF
}

func normalizeTaskAction(input model.TaskActionInput) (model.TaskActionInput, error) {
	input.Comment = strings.TrimSpace(input.Comment)
	if input.Version < 1 || !workflowText(input.Comment, 0, 1000) {
		return input, ErrInvalid
	}
	switch input.Action {
	case model.ActionApprove:
		if input.TargetUserID == nil {
			return input, nil
		}
	case model.ActionReject:
		if input.Comment != "" && input.TargetUserID == nil {
			return input, nil
		}
	case model.ActionTransfer:
		if input.Comment != "" && input.TargetUserID != nil && *input.TargetUserID != 0 {
			return input, nil
		}
	}
	return input, ErrInvalid
}

func workflowText(value string, minimum, maximum int) bool {
	size := utf8.RuneCountInString(value)
	return size >= minimum && size <= maximum && !strings.ContainsRune(value, 0)
}
