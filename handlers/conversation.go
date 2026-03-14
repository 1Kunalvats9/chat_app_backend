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

type ConversationHandler struct {
	cfg *config.Config
}

func NewConversationHandler(cfg *config.Config) *ConversationHandler {
	return &ConversationHandler{cfg: cfg}
}

// Create Direct Conversation
type CreateDirectConversationRequest struct {
	TargetUserID string `json:"target_user_id"`
}

func (h *ConversationHandler) CreateDirect(c *fiber.Ctx) error {
	userID := c.Locals("userID").(uuid.UUID)

	req := new(CreateDirectConversationRequest)
	if err := c.BodyParser(req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	targetID, err := uuid.Parse(req.TargetUserID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid target user ID",
		})
	}

	if userID == targetID {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Cannot create conversation with yourself",
		})
	}

	// Check target user exists
	var targetUser models.User
	if err := database.DB.First(&targetUser, "id = ?", targetID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "User not found",
		})
	}

	// Check if direct conversation already exists between these two users
	var existingConversation models.Conversation
	err = database.DB.
		Joins("JOIN conversation_members cm1 ON cm1.conversation_id = conversations.id AND cm1.user_id = ?", userID).
		Joins("JOIN conversation_members cm2 ON cm2.conversation_id = conversations.id AND cm2.user_id = ?", targetID).
		Where("conversations.type = ?", models.ConversationTypeDirect).
		First(&existingConversation).Error

	if err == nil {
		// Conversation already exists, return it
		database.DB.Preload("Members").First(&existingConversation, "id = ?", existingConversation.ID)
		return c.Status(fiber.StatusOK).JSON(fiber.Map{
			"message": "Conversation already exists",
			"data":    existingConversation,
		})
	}

	// Get current user
	var currentUser models.User
	database.DB.First(&currentUser, "id = ?", userID)

	// Create new conversation
	conversation := models.Conversation{
		Type:    models.ConversationTypeDirect,
		Members: []models.User{currentUser, targetUser},
	}

	if err := database.DB.Create(&conversation).Error; err != nil {
		logger.Log.Error("Failed to create conversation", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Could not create conversation",
		})
	}

	database.DB.Preload("Members").First(&conversation, "id = ?", conversation.ID)

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Conversation created successfully",
		"data":    conversation,
	})
}

// Create Group Conversation
type CreateGroupRequest struct {
	Name      string   `json:"name"`
	MemberIDs []string `json:"member_ids"`
}

func (h *ConversationHandler) CreateGroup(c *fiber.Ctx) error {
	userID := c.Locals("userID").(uuid.UUID)

	req := new(CreateGroupRequest)
	if err := c.BodyParser(req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	if req.Name == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Group name is required",
		})
	}

	if len(req.MemberIDs) < 2 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Group must have at least 2 other members",
		})
	}

	// Build members list starting with creator
	var members []models.User
	var creator models.User
	database.DB.First(&creator, "id = ?", userID)
	members = append(members, creator)

	for _, idStr := range req.MemberIDs {
		memberID, err := uuid.Parse(idStr)
		if err != nil {
			continue
		}
		var member models.User
		if err := database.DB.First(&member, "id = ?", memberID).Error; err == nil {
			members = append(members, member)
		}
	}

	conversation := models.Conversation{
		Type:    models.ConversationTypeGroup,
		Name:    req.Name,
		Members: members,
	}

	if err := database.DB.Create(&conversation).Error; err != nil {
		logger.Log.Error("Failed to create group", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Could not create group",
		})
	}

	database.DB.Preload("Members").First(&conversation, "id = ?", conversation.ID)

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Group created successfully",
		"data":    conversation,
	})
}

// Get All Conversations for current user
func (h *ConversationHandler) GetAll(c *fiber.Ctx) error {
	userID := c.Locals("userID").(uuid.UUID)

	var conversations []models.Conversation

	err := database.DB.
		Joins("JOIN conversation_members ON conversation_members.conversation_id = conversations.id").
		Where("conversation_members.user_id = ?", userID).
		Preload("Members").
		Order("conversations.last_message_at DESC").
		Find(&conversations).Error

	if err != nil {
		logger.Log.Error("Failed to fetch conversations", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Could not fetch conversations",
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"data": conversations,
	})
}

// Get Single Conversation
func (h *ConversationHandler) GetOne(c *fiber.Ctx) error {
	userID := c.Locals("userID").(uuid.UUID)
	conversationID := c.Params("id")

	var conversation models.Conversation

	err := database.DB.
		Joins("JOIN conversation_members ON conversation_members.conversation_id = conversations.id").
		Where("conversations.id = ? AND conversation_members.user_id = ?", conversationID, userID).
		Preload("Members").
		First(&conversation).Error

	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Conversation not found",
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"data": conversation,
	})
}

// Add Member to Group
type AddMemberRequest struct {
	UserID string `json:"user_id"`
}

func (h *ConversationHandler) AddMember(c *fiber.Ctx) error {
	conversationID := c.Params("id")

	req := new(AddMemberRequest)
	if err := c.BodyParser(req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	var conversation models.Conversation
	if err := database.DB.First(&conversation, "id = ?", conversationID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Conversation not found",
		})
	}

	if conversation.Type != models.ConversationTypeGroup {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Cannot add members to a direct conversation",
		})
	}

	memberID, err := uuid.Parse(req.UserID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid user ID",
		})
	}

	var newMember models.User
	if err := database.DB.First(&newMember, "id = ?", memberID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "User not found",
		})
	}

	if err := database.DB.Model(&conversation).Association("Members").Append(&newMember); err != nil {
		logger.Log.Error("Failed to add member", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Could not add member",
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Member added successfully",
	})
}