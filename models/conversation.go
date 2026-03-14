package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ConversationType string 

const (
	ConversationTypeDirect ConversationType = "direct"
	ConversationTypeGroup  ConversationType = "group"
)


type Conversation struct {
	ID           uuid.UUID        `gorm:"type:uuid;primaryKey" json:"id"`
	Type         ConversationType `gorm:"type:varchar(20);default:'direct'" json:"type"`
	Name         string           `json:"name"`
	Avatar       string           `gorm:"type:text" json:"avatar"`
	LastMessage  string           `gorm:"type:text" json:"last_message"`
	LastMessageAt time.Time       `json:"last_message_at"`
	CreatedAt    time.Time        `json:"created_at"`
	UpdatedAt    time.Time        `json:"updated_at"`
	DeletedAt    gorm.DeletedAt   `gorm:"index" json:"-"`

	Members  []User    `gorm:"many2many:conversation_members;" json:"members,omitempty"`
}

func (c *Conversation) BeforeCreate(tx *gorm.DB) error {
	c.ID = uuid.New() 
	return nil 
}