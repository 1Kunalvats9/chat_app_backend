package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type MediaType string

const (
	MediaTypeImage    MediaType = "image"
	MediaTypeVideo    MediaType = "video"
	MediaTypeDocument MediaType = "document"
	MediaTypeAudio    MediaType = "audio"
)

type Media struct {
	ID        uuid.UUID      `gorm:"type:uuid;primaryKey" json:"id"`
	MessageID uuid.UUID      `gorm:"type:uuid;not null;index" json:"message_id"`
	UploaderID uuid.UUID     `gorm:"type:uuid;not null" json:"uploader_id"`
	Type      MediaType      `gorm:"type:varchar(20)" json:"type"`
	URL       string         `gorm:"type:text;not null" json:"url"`
	FileName  string         `gorm:"not null" json:"file_name"`
	FileSize  int64          `json:"file_size"`
	MimeType  string         `gorm:"not null" json:"mime_type"`
	Width     int            `json:"width"`
	Height    int            `json:"height"`
	Duration  int            `json:"duration"`
	CreatedAt time.Time      `json:"created_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

func (m *Media) BeforeCreate(tx *gorm.DB) error {
	m.ID = uuid.New()
	return nil
}