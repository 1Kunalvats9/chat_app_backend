package routes

import (
	"chat-backend/config"
	"chat-backend/handlers"
	"chat-backend/middleware"
	"chat-backend/utils"
	"chat-backend/websocket"

	"github.com/gofiber/fiber/v2"
	fiberws "github.com/gofiber/websocket/v2"
)

func Setup(app *fiber.App, cfg *config.Config, hub *websocket.Hub) {
	authHandler := handlers.NewAuthHandler(cfg)
	convHandler := handlers.NewConversationHandler(cfg)
	msgHandler := handlers.NewMessageHandler(cfg)
	mediaHandler := handlers.NewMediaHandler(cfg)

	api := app.Group("/api")
	v1 := api.Group("/v1")

	// Public routes
	auth := v1.Group("/auth")
	auth.Post("/register", authHandler.Register)
	auth.Post("/login", authHandler.Login)
	auth.Post("/refresh", authHandler.RefreshToken)

	// Protected routes
	protected := v1.Group("", middleware.AuthRequired(cfg))

	// Auth
	protected.Post("/auth/logout", authHandler.Logout)

	// Conversations
	conversations := protected.Group("/conversations")
	conversations.Get("/", convHandler.GetAll)
	conversations.Post("/direct", convHandler.CreateDirect)
	conversations.Post("/group", convHandler.CreateGroup)
	conversations.Get("/:id", convHandler.GetOne)
	conversations.Post("/:id/members", convHandler.AddMember)

	// Messages
	conversations.Get("/:id/messages", msgHandler.GetMessages)
	conversations.Post("/:id/messages", msgHandler.SendMessage)

	// Message actions
	messages := protected.Group("/messages")
	messages.Put("/:id", msgHandler.EditMessage)
	messages.Delete("/:id", msgHandler.DeleteMessage)
	messages.Patch("/:id/status", msgHandler.UpdateStatus)

	// Media
	media := protected.Group("/media")
	media.Post("/presign", mediaHandler.GetPresignedURL)
	media.Post("/confirm", mediaHandler.ConfirmUpload)
	media.Post("/upload", mediaHandler.UploadDirect)

	// WebSocket
	app.Use("/ws", func(c *fiber.Ctx) error {
		token := c.Query("token")
		if token == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Token is required",
			})
		}
		if fiberws.IsWebSocketUpgrade(c) {
			c.Locals("token", token)
			return c.Next()
		}
		return fiber.ErrUpgradeRequired
	})

	app.Get("/ws", fiberws.New(func(c *fiberws.Conn) {
		tokenStr := c.Locals("token").(string)
		claims, err := utils.ValidateAccessToken(cfg, tokenStr)
		if err != nil {
			c.Close()
			return
		}
		hub.HandleClient(c, claims.UserID)
	}))
}