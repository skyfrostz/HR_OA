package model

import (
	"encoding/json"
	"time"
)

type RouteStep struct {
	Step         int    `json:"step"`
	Name         string `json:"name"`
	AssigneeID   uint   `json:"assigneeId"`
	AssigneeName string `json:"assigneeName"`
}

type RequestTemplate struct {
	Type        string      `json:"type"`
	Name        string      `json:"name"`
	Version     int         `json:"version"`
	Description string      `json:"description"`
	Route       []RouteStep `json:"route"`
}

type TemplatesView struct {
	List            []RequestTemplate `json:"list"`
	IntegrationMode string            `json:"integrationMode"`
}

type SubmitInput struct {
	Type              string          `json:"type"`
	Title             string          `json:"title"`
	Form              json.RawMessage `json:"form" swaggertype:"object"`
	PreviousRequestID *uint           `json:"previousRequestId"`
}

type TaskActionInput struct {
	Action       string `json:"action"`
	Version      int    `json:"version"`
	Comment      string `json:"comment"`
	TargetUserID *uint  `json:"targetUserId"`
}

type RequestSummary struct {
	ID                uint      `json:"id"`
	Type              string    `json:"type"`
	Title             string    `json:"title"`
	Status            string    `json:"status"`
	ApplicantID       uint      `json:"applicantId"`
	ApplicantName     string    `json:"applicantName"`
	CreatedAt         time.Time `json:"createdAt"`
	UpdatedAt         time.Time `json:"updatedAt"`
	PreviousRequestID *uint     `json:"previousRequestId"`
	CurrentStep       *int      `json:"currentStep"`
}

type RequestDetail struct {
	LeaveCalculation *LeaveCalculation `json:"leaveCalculation"`
	Rule             *RuleReference    `json:"rule"`

	RequestSummary
	TemplateVersion int             `json:"templateVersion"`
	Form            json.RawMessage `json:"form" swaggertype:"object"`
	Route           []RouteStep     `json:"route"`
	Tasks           []TaskView      `json:"tasks"`
	History         []ActionView    `json:"history"`
}

type TaskView struct {
	ID            uint       `json:"id"`
	RequestID     uint       `json:"requestId"`
	RequestTitle  string     `json:"requestTitle"`
	RequestType   string     `json:"requestType"`
	ApplicantName string     `json:"applicantName"`
	Step          int        `json:"step"`
	StepName      string     `json:"stepName"`
	AssigneeID    uint       `json:"assigneeId"`
	AssigneeName  string     `json:"assigneeName"`
	Status        string     `json:"status"`
	Version       int        `json:"version"`
	CreatedAt     time.Time  `json:"createdAt"`
	CompletedAt   *time.Time `json:"completedAt"`
}

type ActionView struct {
	RouteStep          *int      `json:"routeStep,omitempty"`
	PreviousAssigneeID *uint     `json:"previousAssigneeId,omitempty"`
	ID                 uint      `json:"id"`
	RequestID          uint      `json:"requestId"`
	RequestTitle       string    `json:"requestTitle"`
	RequestType        string    `json:"requestType"`
	TaskID             *uint     `json:"taskId"`
	ActorID            uint      `json:"actorId"`
	ActorName          string    `json:"actorName"`
	Action             string    `json:"action"`
	Comment            string    `json:"comment"`
	TargetUserID       *uint     `json:"targetUserId"`
	TargetName         *string   `json:"targetName"`
	CreatedAt          time.Time `json:"createdAt"`
}

type TransferTarget struct {
	ID          uint   `json:"id"`
	DisplayName string `json:"displayName"`
}

type TransferTargetsView struct {
	List []TransferTarget `json:"list"`
}

type NotificationView struct {
	ID           uint      `json:"id"`
	EventID      uint      `json:"eventId"`
	RequestID    uint      `json:"requestId"`
	Kind         string    `json:"kind"`
	Title        string    `json:"title"`
	CreatedAt    time.Time `json:"createdAt"`
	DeliveryMode string    `json:"deliveryMode"`
}
