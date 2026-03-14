package websocket

import (
	"chat-backend/database"
	"chat-backend/logger"
	"chat-backend/models"
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/gofiber/websocket/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// Event types
const (
	EventNewMessage     = "new_message"
	EventMessageStatus  = "message_status"
	EventTyping         = "typing"
	EventStopTyping     = "stop_typing"
	EventUserOnline     = "user_online"
	EventUserOffline    = "user_offline"
)

// Incoming event from client
type IncomingEvent struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// Outgoing event to client
type OutgoingEvent struct {
	Type      string      `json:"type"`
	Payload   interface{} `json:"payload"`
	CreatedAt time.Time   `json:"created_at"`
}

// Typing payload
type TypingPayload struct {
	ConversationID string `json:"conversation_id"`
	UserID         string `json:"user_id"`
	Username       string `json:"username"`
}

// Message status payload
type StatusPayload struct {
	MessageID string `json:"message_id"`
	Status    string `json:"status"`
}

// Client represents a connected websocket user
type Client struct {
	ID     uuid.UUID
	Conn   *websocket.Conn
	Send   chan []byte
	Hub    *Hub
}

// Hub manages all connected clients
type Hub struct {
	Clients    map[uuid.UUID]*Client
	Register   chan *Client
	Unregister chan *Client
	Broadcast  chan []byte
	mu         sync.RWMutex
	redis      *redis.Client
}

func NewHub(redisClient *redis.Client) *Hub {
	return &Hub{
		Clients:    make(map[uuid.UUID]*Client),
		Register:   make(chan *Client),
		Unregister: make(chan *Client),
		Broadcast:  make(chan []byte),
		redis:      redisClient,
	}
}

// Run starts the hub event loop
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.Register:
			h.mu.Lock()
			h.Clients[client.ID] = client
			h.mu.Unlock()

			// Mark user online in DB and Redis
			database.DB.Model(&models.User{}).Where("id = ?", client.ID).Updates(map[string]interface{}{
				"is_online": true,
				"last_seen": time.Now(),
			})
			h.redis.Set(context.Background(), "online:"+client.ID.String(), "1", 24*time.Hour)

			logger.Log.Info("Client connected", zap.String("userID", client.ID.String()))

			// Notify others this user is online
			h.broadcastToOthers(client.ID, OutgoingEvent{
				Type:      EventUserOnline,
				Payload:   map[string]string{"user_id": client.ID.String()},
				CreatedAt: time.Now(),
			})

		case client := <-h.Unregister:
			h.mu.Lock()
			if _, ok := h.Clients[client.ID]; ok {
				delete(h.Clients, client.ID)
				close(client.Send)
			}
			h.mu.Unlock()

			// Mark user offline
			database.DB.Model(&models.User{}).Where("id = ?", client.ID).Updates(map[string]interface{}{
				"is_online": false,
				"last_seen": time.Now(),
			})
			h.redis.Del(context.Background(), "online:"+client.ID.String())

			logger.Log.Info("Client disconnected", zap.String("userID", client.ID.String()))

			// Notify others this user is offline
			h.broadcastToOthers(client.ID, OutgoingEvent{
				Type:      EventUserOffline,
				Payload:   map[string]string{"user_id": client.ID.String()},
				CreatedAt: time.Now(),
			})

		case message := <-h.Broadcast:
			h.mu.RLock()
			for _, client := range h.Clients {
				select {
				case client.Send <- message:
				default:
					close(client.Send)
					delete(h.Clients, client.ID)
				}
			}
			h.mu.RUnlock()
		}
	}
}

// SendToUser sends event to a specific user if they are connected
func (h *Hub) SendToUser(userID uuid.UUID, event OutgoingEvent) {
	h.mu.RLock()
	client, ok := h.Clients[userID]
	h.mu.RUnlock()

	if !ok {
		return
	}

	data, err := json.Marshal(event)
	if err != nil {
		return
	}

	select {
	case client.Send <- data:
	default:
		h.mu.Lock()
		close(client.Send)
		delete(h.Clients, userID)
		h.mu.Unlock()
	}
}

// SendToConversation sends event to all members of a conversation
func (h *Hub) SendToConversation(conversationID uuid.UUID, senderID uuid.UUID, event OutgoingEvent) {
	var members []models.User
	database.DB.
		Joins("JOIN conversation_members ON conversation_members.user_id = users.id").
		Where("conversation_members.conversation_id = ?", conversationID).
		Find(&members)

	for _, member := range members {
		if member.ID == senderID {
			continue
		}
		h.SendToUser(member.ID, event)
	}
}

// broadcastToOthers sends event to all connected clients except the sender
func (h *Hub) broadcastToOthers(senderID uuid.UUID, event OutgoingEvent) {
	data, err := json.Marshal(event)
	if err != nil {
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	for id, client := range h.Clients {
		if id == senderID {
			continue
		}
		select {
		case client.Send <- data:
		default:
		}
	}
}

// IsOnline checks if a user is online via Redis
func (h *Hub) IsOnline(userID uuid.UUID) bool {
	result := h.redis.Get(context.Background(), "online:"+userID.String())
	return result.Err() == nil
}

// HandleClient manages a single websocket connection
func (h *Hub) HandleClient(c *websocket.Conn, userID uuid.UUID) {
	client := &Client{
		ID:   userID,
		Conn: c,
		Send: make(chan []byte, 256),
		Hub:  h,
	}

	h.Register <- client

	// Start writer goroutine
	go client.writePump()

	// Read pump runs in current goroutine
	client.readPump()
}

// readPump reads incoming messages from websocket
func (c *Client) readPump() {
	defer func() {
		c.Hub.Unregister <- c
		c.Conn.Close()
	}()

	c.Conn.SetReadLimit(512 * 1024) // 512KB max message size

	for {
		_, msg, err := c.Conn.ReadMessage()
		if err != nil {
			break
		}

		var event IncomingEvent
		if err := json.Unmarshal(msg, &event); err != nil {
			continue
		}

		c.handleEvent(event)
	}
}

// writePump writes outgoing messages to websocket
func (c *Client) writePump() {
	ticker := time.NewTicker(30 * time.Second)
	defer func() {
		ticker.Stop()
		c.Conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.Send:
			if !ok {
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			c.Conn.WriteMessage(websocket.TextMessage, message)

		case <-ticker.C:
			// Send ping every 30 seconds to keep connection alive
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// handleEvent processes incoming events from client
func (c *Client) handleEvent(event IncomingEvent) {
	switch event.Type {

	case EventTyping:
		var payload TypingPayload
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return
		}
		convID, err := uuid.Parse(payload.ConversationID)
		if err != nil {
			return
		}
		payload.UserID = c.ID.String()

		// Get username
		var user models.User
		database.DB.First(&user, "id = ?", c.ID)
		payload.Username = user.Username

		c.Hub.SendToConversation(convID, c.ID, OutgoingEvent{
			Type:      EventTyping,
			Payload:   payload,
			CreatedAt: time.Now(),
		})

	case EventStopTyping:
		var payload TypingPayload
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return
		}
		convID, err := uuid.Parse(payload.ConversationID)
		if err != nil {
			return
		}
		payload.UserID = c.ID.String()

		c.Hub.SendToConversation(convID, c.ID, OutgoingEvent{
			Type:      EventStopTyping,
			Payload:   payload,
			CreatedAt: time.Now(),
		})

	case EventMessageStatus:
		var payload StatusPayload
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return
		}

		msgID, err := uuid.Parse(payload.MessageID)
		if err != nil {
			return
		}

		var message models.Message
		if err := database.DB.First(&message, "id = ?", msgID).Error; err != nil {
			return
		}

		database.DB.Model(&message).Update("status", payload.Status)

		// Notify sender their message was delivered/read
		c.Hub.SendToUser(message.SenderID, OutgoingEvent{
			Type:      EventMessageStatus,
			Payload:   payload,
			CreatedAt: time.Now(),
		})
	}
}