package model

import "time"

const (
	FileStandard   = "STANDARD"
	FileRestricted = "RESTRICTED"
	StorageLocal   = "local"
	StorageOSS     = "oss"
	ImportExpired  = "EXPIRED"
)

type AttachmentClassification struct {
	Classification string `json:"classification"`
	Version        int    `json:"version"`
	Reason         string `json:"reason"`
}

type ImportHistoryItem struct {
	ID            string     `json:"id"`
	Status        string     `json:"status"`
	CreatedAt     time.Time  `json:"createdAt"`
	ExpiresAt     time.Time  `json:"expiresAt"`
	ConfirmedAt   *time.Time `json:"confirmedAt"`
	TotalRows     int        `json:"totalRows"`
	ValidRows     int        `json:"validRows"`
	ErrorRows     int        `json:"errorRows"`
	ImportedCount *int       `json:"importedCount"`
}

type IntegrationStatus struct {
	WeComLogin        string `json:"wecomLogin"`
	Notifications     string `json:"notifications"`
	AttachmentStorage string `json:"attachmentStorage"`
}

type WeComBinding struct {
	EmployeeID uint   `gorm:"primaryKey;autoIncrement:false"`
	UserID     uint   `gorm:"not null;uniqueIndex"`
	Mode       string `gorm:"size:8;not null;uniqueIndex:ux_oa_wecom_identity,priority:1"`
	CorpID     string `gorm:"type:varbinary(64);not null;uniqueIndex:ux_oa_wecom_identity,priority:2"`
	MemberID   string `gorm:"type:varbinary(64);not null;uniqueIndex:ux_oa_wecom_identity,priority:3"`
}

func (WeComBinding) TableName() string { return "oa_wecom_bindings" }

type WeComChallenge struct {
	Hash         string    `gorm:"type:char(64);primaryKey"`
	Kind         string    `gorm:"size:8;not null"`
	Mode         string    `gorm:"size:8;not null"`
	CorpID       string    `gorm:"type:varbinary(64);not null"`
	Entry        string    `gorm:"size:3;not null"`
	UserID       uint      `gorm:"not null"`
	SessionEpoch uint64    `gorm:"not null"`
	ExpiresAt    time.Time `gorm:"not null;index"`
}

func (WeComChallenge) TableName() string { return "oa_wecom_challenges" }

type WeComBindingView struct {
	EmployeeID uint    `json:"employeeId"`
	Version    int     `json:"version"`
	MemberID   *string `json:"memberId"`
}

type WeComBindingInput struct {
	Version  int     `json:"version"`
	MemberID *string `json:"memberId"`
	Reason   string  `json:"reason"`
}

type NotificationDeliveryView struct {
	ID        uint      `json:"id"`
	Kind      string    `json:"kind"`
	Mode      string    `json:"mode"`
	Status    string    `json:"status"`
	Attempts  uint      `json:"attempts"`
	CreatedAt time.Time `json:"createdAt"`
}
