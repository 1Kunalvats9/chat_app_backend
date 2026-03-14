package handlers

import (
	"chat-backend/config"
	"chat-backend/database"
	"chat-backend/logger"
	"chat-backend/models"
	"chat-backend/utils"
	"path/filepath"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type MediaHandler struct {
	cfg *config.Config
}

func NewMediaHandler(cfg *config.Config) *MediaHandler {
	return &MediaHandler{cfg: cfg}
}

var allowedMimeTypes = map[string]models.MediaType{
	"image/jpeg":      models.MediaTypeImage,
	"image/png":       models.MediaTypeImage,
	"image/gif":       models.MediaTypeImage,
	"image/webp":      models.MediaTypeImage,
	"video/mp4":       models.MediaTypeVideo,
	"video/quicktime": models.MediaTypeVideo,
	"video/webm":      models.MediaTypeVideo,
	"audio/mpeg":      models.MediaTypeAudio,
	"audio/ogg":       models.MediaTypeAudio,
	"audio/wav":       models.MediaTypeAudio,
	"application/pdf": models.MediaTypeDocument,
	"application/msword":                                            models.MediaTypeDocument,
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document": models.MediaTypeDocument,
}

var maxFileSizes = map[models.MediaType]int64{
	models.MediaTypeImage:    10 * 1024 * 1024,  // 10MB
	models.MediaTypeVideo:    100 * 1024 * 1024, // 100MB
	models.MediaTypeAudio:    20 * 1024 * 1024,  // 20MB
	models.MediaTypeDocument: 30 * 1024 * 1024,  // 30MB
}

// Get Presigned URL for direct upload from client
type PresignRequest struct {
	FileName string `json:"file_name"`
	MimeType string `json:"mime_type"`
	FileSize int64  `json:"file_size"`
}

func (h *MediaHandler) GetPresignedURL(c *fiber.Ctx) error {
	req := new(PresignRequest)
	if err := c.BodyParser(req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	mediaType, allowed := allowedMimeTypes[req.MimeType]
	if !allowed {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "File type not allowed",
		})
	}

	maxSize := maxFileSizes[mediaType]
	if req.FileSize > maxSize {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "File size exceeds limit",
		})
	}

	ext := filepath.Ext(req.FileName)
	folder := string(mediaType) + "s"
	key := folder + "/" + uuid.New().String() + ext

	presignedURL, err := utils.GeneratePresignedURL(c.Context(), key, 15*60*1000000000)
	if err != nil {
		logger.Log.Error("Failed to generate presigned URL", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Could not generate upload URL",
		})
	}

	finalURL := utils.R2PublicURL + "/" + key

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"data": fiber.Map{
			"upload_url": presignedURL,
			"final_url":  finalURL,
			"key":        key,
			"expires_in": 900,
		},
	})
}

// Confirm upload and attach media to message
type ConfirmUploadRequest struct {
	ConversationID string  `json:"conversation_id"`
	Key            string  `json:"key"`
	FileName       string  `json:"file_name"`
	FileSize       int64   `json:"file_size"`
	MimeType       string  `json:"mime_type"`
	Width          int     `json:"width"`
	Height         int     `json:"height"`
	Duration       int     `json:"duration"`
	Caption        string  `json:"caption"`
	ReplyToID      *string `json:"reply_to_id"`
}

func (h *MediaHandler) ConfirmUpload(c *fiber.Ctx) error {
	userID := c.Locals("userID").(uuid.UUID)

	req := new(ConfirmUploadRequest)
	if err := c.BodyParser(req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Verify user is in conversation
	convID, err := uuid.Parse(req.ConversationID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid conversation ID",
		})
	}

	var conversation models.Conversation
	err = database.DB.
		Joins("JOIN conversation_members ON conversation_members.conversation_id = conversations.id").
		Where("conversations.id = ? AND conversation_members.user_id = ?", convID, userID).
		First(&conversation).Error

	if err != nil {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": "You are not part of this conversation",
		})
	}

	mediaType := allowedMimeTypes[req.MimeType]
	finalURL := utils.R2PublicURL + "/" + req.Key

	// Determine message type from media type
	var msgType models.MessageType
	switch mediaType {
	case models.MediaTypeImage:
		msgType = models.MessageTypeImage
	case models.MediaTypeVideo:
		msgType = models.MessageTypeVideo
	case models.MediaTypeAudio:
		msgType = models.MessageTypeAudio
	default:
		msgType = models.MessageTypeDocument
	}

	// Create message
	message := models.Message{
		ConversationID: convID,
		SenderID:       userID,
		Type:           msgType,
		Content:        req.Caption,
		Status:         models.MessageStatusSent,
	}

	if req.ReplyToID != nil {
		replyID, err := uuid.Parse(*req.ReplyToID)
		if err == nil {
			message.ReplyToID = &replyID
		}
	}

	if err := database.DB.Create(&message).Error; err != nil {
		logger.Log.Error("Failed to create message", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Could not create message",
		})
	}

	// Create media record
	media := models.Media{
		MessageID:  message.ID,
		UploaderID: userID,
		Type:       mediaType,
		URL:        finalURL,
		FileName:   req.FileName,
		FileSize:   req.FileSize,
		MimeType:   req.MimeType,
		Width:      req.Width,
		Height:     req.Height,
		Duration:   req.Duration,
	}

	if err := database.DB.Create(&media).Error; err != nil {
		logger.Log.Error("Failed to create media record", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Could not save media",
		})
	}

	// Update conversation last message
	lastMsg := strings.ToUpper(string(mediaType))
	database.DB.Model(&conversation).Updates(map[string]interface{}{
		"last_message":    lastMsg,
		"last_message_at": message.CreatedAt,
	})

	// Load full message with relations
	database.DB.
		Preload("Sender").
		Preload("Media").
		Preload("ReplyTo").
		Preload("ReplyTo.Sender").
		First(&message, "id = ?", message.ID)

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Media sent successfully",
		"data":    message,
	})
}

// Direct upload through server (fallback)
func (h *MediaHandler) UploadDirect(c *fiber.Ctx) error {
	userID := c.Locals("userID").(uuid.UUID)
	_ = userID

	file, err := c.FormFile("file")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "No file provided",
		})
	}

	mimeType := file.Header.Get("Content-Type")
	mediaType, allowed := allowedMimeTypes[mimeType]
	if !allowed {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "File type not allowed",
		})
	}

	maxSize := maxFileSizes[mediaType]
	if file.Size > maxSize {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "File size exceeds limit",
		})
	}

	folder := string(mediaType) + "s"
	result, err := utils.UploadFile(c.Context(), file, folder)
	if err != nil {
		logger.Log.Error("Failed to upload file", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Could not upload file",
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"data": fiber.Map{
			"url":       result.URL,
			"file_name": result.FileName,
			"file_size": result.FileSize,
			"mime_type": result.MimeType,
		},
	})
}