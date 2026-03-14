package handlers

import (
	"chat-backend/config"
	"chat-backend/database"
	"chat-backend/logger"
	"chat-backend/models"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type MessageHandler struct {
	cfg *config.Config
}

func NewMessageHandler(cfg *config.Config) *MessageHandler {
	return &MessageHandler{cfg: cfg}
}

// Send Text Message
type SendMessageRequest struct {
	Content    string  `json:"content"`
	ReplyToID  *string `json:"reply_to_id"`
}

func (h *MessageHandler) SendMessage(c *fiber.Ctx) error {
	userID := c.Locals("userID").(uuid.UUID)
	conversationID := c.Params("id")

	// Verify user is part of this conversation
	var conversation models.Conversation
	err := database.DB.
		Joins("JOIN conversation_members ON conversation_members.conversation_id = conversations.id").
		Where("conversations.id = ? AND conversation_members.user_id = ?", conversationID, userID).
		First(&conversation).Error

	if err != nil {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": "You are not part of this conversation",
		})
	}

	req := new(SendMessageRequest)
	if err := c.BodyParser(req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	if req.Content == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Message content is required",
		})
	}

	convID, _ := uuid.Parse(conversationID)

	message := models.Message{
		ConversationID: convID,
		SenderID:       userID,
		Type:           models.MessageTypeText,
		Content:        req.Content,
		Status:         models.MessageStatusSent,
	}

	// Handle reply
	if req.ReplyToID != nil {
		replyID, err := uuid.Parse(*req.ReplyToID)
		if err == nil {
			message.ReplyToID = &replyID
		}
	}

	if err := database.DB.Create(&message).Error; err != nil {
		logger.Log.Error("Failed to create message", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Could not send message",
		})
	}

	// Update conversation last message
	database.DB.Model(&conversation).Updates(map[string]interface{}{
		"last_message":    req.Content,
		"last_message_at": message.CreatedAt,
	})

	// Load sender info
	database.DB.Preload("Sender").Preload("ReplyTo").First(&message, "id = ?", message.ID)

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Message sent successfully",
		"data":    message,
	})
}

// Get Messages in a Conversation (paginated)
func (h *MessageHandler) GetMessages(c *fiber.Ctx) error {
	userID := c.Locals("userID").(uuid.UUID)
	conversationID := c.Params("id")

	// Verify user is part of this conversation
	var conversation models.Conversation
	err := database.DB.
		Joins("JOIN conversation_members ON conversation_members.conversation_id = conversations.id").
		Where("conversations.id = ? AND conversation_members.user_id = ?", conversationID, userID).
		First(&conversation).Error

	if err != nil {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": "You are not part of this conversation",
		})
	}

	// Pagination
	page := c.QueryInt("page", 1)
	limit := c.QueryInt("limit", 30)
	offset := (page - 1) * limit

	var messages []models.Message
	var total int64

	database.DB.Model(&models.Message{}).
		Where("conversation_id = ?", conversationID).
		Count(&total)

	err = database.DB.
		Where("conversation_id = ?", conversationID).
		Preload("Sender").
		Preload("Media").
		Preload("ReplyTo").
		Preload("ReplyTo.Sender").
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&messages).Error

	if err != nil {
		logger.Log.Error("Failed to fetch messages", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Could not fetch messages",
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"data": fiber.Map{
			"messages": messages,
			"pagination": fiber.Map{
				"total": total,
				"page":  page,
				"limit": limit,
				"pages": (total + int64(limit) - 1) / int64(limit),
			},
		},
	})
}

// Update Message Status (delivered/read)
type UpdateStatusRequest struct {
	Status string `json:"status"`
}

func (h *MessageHandler) UpdateStatus(c *fiber.Ctx) error {
	userID := c.Locals("userID").(uuid.UUID)
	messageID := c.Params("id")

	var message models.Message
	if err := database.DB.First(&message, "id = ?", messageID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Message not found",
		})
	}

	// Only recipient can update status, not the sender
	if message.SenderID == userID {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": "Cannot update status of your own message",
		})
	}

	req := new(UpdateStatusRequest)
	if err := c.BodyParser(req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	validStatuses := map[string]bool{
		"delivered": true,
		"read":      true,
	}

	if !validStatuses[req.Status] {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid status. Use: delivered or read",
		})
	}

	database.DB.Model(&message).Update("status", req.Status)

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Status updated successfully",
	})
}

// Edit Message
type EditMessageRequest struct {
	Content string `json:"content"`
}

func (h *MessageHandler) EditMessage(c *fiber.Ctx) error {
	userID := c.Locals("userID").(uuid.UUID)
	messageID := c.Params("id")

	var message models.Message
	if err := database.DB.First(&message, "id = ?", messageID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Message not found",
		})
	}

	// Only sender can edit their own message
	if message.SenderID != userID {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": "You can only edit your own messages",
		})
	}

	if message.Type != models.MessageTypeText {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Only text messages can be edited",
		})
	}

	req := new(EditMessageRequest)
	if err := c.BodyParser(req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	if req.Content == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Content cannot be empty",
		})
	}

	database.DB.Model(&message).Updates(map[string]interface{}{
		"content":   req.Content,
		"is_edited": true,
	})

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Message edited successfully",
		"data":    message,
	})
}

// Delete Message
func (h *MessageHandler) DeleteMessage(c *fiber.Ctx) error {
	userID := c.Locals("userID").(uuid.UUID)
	messageID := c.Params("id")

	var message models.Message
	if err := database.DB.First(&message, "id = ?", messageID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Message not found",
		})
	}

	if message.SenderID != userID {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": "You can only delete your own messages",
		})
	}

	// Soft delete the message
	if err := database.DB.Delete(&message).Error; err != nil {
		logger.Log.Error("Failed to delete message", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Could not delete message",
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Message deleted successfully",
	})
}