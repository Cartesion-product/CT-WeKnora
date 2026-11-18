package types

import "time"

type VideoUpload struct {
	ID                string    `json:"upload_id" gorm:"type:varchar(36);primaryKey"`
	TenantID          uint64    `json:"-"`
	KnowledgeBaseID   string    `json:"-" gorm:"type:varchar(36)"`
	FileName          string    `json:"-" gorm:"type:varchar(255)"`
	Size              int64     `json:"-"`
	MIME              string    `json:"-" gorm:"type:varchar(100)"`
	FolderPath        string    `json:"-" gorm:"type:varchar(1024)"`
	TagIDsJSON        string    `json:"-" gorm:"type:text"`
	ProcessConfigJSON string    `json:"-" gorm:"type:text"`
	Status            string    `json:"status" gorm:"type:varchar(32)"`
	KnowledgeID       string    `json:"knowledge_id,omitempty" gorm:"type:varchar(36)"`
	CreatedAt         time.Time `json:"-"`
	UpdatedAt         time.Time `json:"-"`
}

type VideoUploadPart struct {
	UploadID   string `gorm:"type:varchar(36);primaryKey"`
	PartNumber int    `gorm:"primaryKey"`
	Size       int64
	FilePath   string `gorm:"type:text"`
}
