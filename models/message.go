package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type MessageType string
type MessageStatus string

const (
	MessageTypeText     MessageType = "text"
	MessageTypeImage    MessageType = "image"
	MessageTypeVideo    MessageType = "video"
	MessageTypeDocument MessageType = "document"
	MessageTypeAudio    MessageType = "audio"

	MessageStatusSent      MessageStatus = "sent"
	MessageStatusDelivered MessageStatus = "delivered"
	MessageStatusRead      MessageStatus = "read"
)

type Message struct {
	ID             uuid.UUID     `gorm:"type:uuid;primaryKey" json:"id"`
	ConversationID uuid.UUID     `gorm:"type:uuid;not null;index" json:"conversation_id"`
	SenderID       uuid.UUID     `gorm:"type:uuid;not null;index" json:"sender_id"`
	Type           MessageType   `gorm:"type:varchar(20);default:'text'" json:"type"`
	Content        string        `gorm:"type:text" json:"content"`
	Status         MessageStatus `gorm:"type:varchar(20);default:'sent'" json:"status"`
	ReplyToID      *uuid.UUID    `gorm:"type:uuid" json:"reply_to_id"`
	IsEdited       bool          `gorm:"default:false" json:"is_edited"`
	CreatedAt      time.Time     `json:"created_at"`
	UpdatedAt      time.Time     `json:"updated_at"`
	DeletedAt      gorm.DeletedAt `gorm:"index" json:"-"`

	Sender       User         `gorm:"foreignKey:SenderID" json:"sender,omitempty"`
	Conversation Conversation `gorm:"foreignKey:ConversationID" json:"-"`
	Media        []Media      `gorm:"foreignKey:MessageID" json:"media,omitempty"`
	ReplyTo      *Message     `gorm:"foreignKey:ReplyToID" json:"reply_to,omitempty"`
}

func (m *Message) BeforeCreate(tx *gorm.DB) error {
	m.ID = uuid.New()
	return nil
}