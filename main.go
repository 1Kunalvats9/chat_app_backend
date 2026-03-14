package main

import (
	"chat-backend/config"
	"chat-backend/database"
	"chat-backend/logger"
	"chat-backend/models"
	"chat-backend/routes"
	"chat-backend/utils"
	"chat-backend/websocket"
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

func main() {
	// Load config
	cfg := config.Load()

	// Init logger
	logger.Init(cfg.AppEnv)
	defer logger.Sync()

	// Connect database
	database.Connect(cfg)
	database.AutoMigrate(
		&models.User{},
		&models.Conversation{},
		&models.Message{},
		&models.Media{},
	)

	// Init R2
	if err := utils.InitR2(cfg); err != nil {
		logger.Log.Fatal("Failed to initialize R2", zap.Error(err))
	}
	logger.Log.Info("R2 storage initialized")

	// Init Redis
	redisClient := redis.NewClient(&redis.Options{
		Addr: cfg.RedisURL,
	})
	if err := redisClient.Ping(context.Background()).Err(); err != nil {
		logger.Log.Fatal("Failed to connect to Redis", zap.Error(err))
	}
	logger.Log.Info("Redis connected")

	// Init WebSocket hub
	hub := websocket.NewHub(redisClient)
	go hub.Run()
	logger.Log.Info("WebSocket hub started")

	// Init Fiber
	app := fiber.New(fiber.Config{
		AppName:   cfg.AppName,
		BodyLimit: 110 * 1024 * 1024, // 110MB to handle video uploads
	})

	// CORS middleware
	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowHeaders: "Origin, Content-Type, Accept, Authorization",
		AllowMethods: "GET, POST, PUT, PATCH, DELETE",
	}))

	// Setup routes
	routes.Setup(app, cfg, hub)

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-quit
		logger.Log.Info("Shutting down server...")

		// Mark all users offline on shutdown
		database.DB.Model(&models.User{}).Where("is_online = ?", true).Update("is_online", false)
		redisClient.FlushDB(context.Background())

		if err := app.Shutdown(); err != nil {
			logger.Log.Fatal("Error during shutdown", zap.Error(err))
		}
	}()

	logger.Log.Info("Server starting", zap.String("port", cfg.AppPort))
	if err := app.Listen(":" + cfg.AppPort); err != nil {
		logger.Log.Fatal("Server failed to start", zap.Error(err))
	}
}