package bridge

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/open-agents/bridge/internal/config"
	"github.com/open-agents/bridge/internal/crypto"
	"github.com/open-agents/bridge/internal/permission"
	"github.com/open-agents/bridge/internal/session"
	"github.com/open-agents/bridge/internal/storage"
)

type Bridge struct {
	config      *config.Config
	conn        *websocket.Conn
	sessions    *session.Manager
	permServer  *permission.Server
	permHandler *permission.Handler
	store       *storage.Store
	keyPair     *crypto.KeyPair
	webPubKey   *[crypto.KeySize]byte
	done        chan struct{}
	mu          sync.Mutex
}

func New(cfg *config.Config) (*Bridge, error) {
	handler := permission.NewHandler()

	// Initialize storage
	storeDir := filepath.Join(config.ConfigDir(), "sessions")
	store, _ := storage.NewStore(storeDir)

	b := &Bridge{
		config:      cfg,
		sessions:    session.NewManager(),
		permHandler: handler,
		permServer:  permission.NewServer(handler),
		store:       store,
		done:        make(chan struct{}),
	}

	// Load E2EE keys if available
	if cfg.PrivateKey != "" {
		privBytes, err := base64.StdEncoding.DecodeString(cfg.PrivateKey)
		if err == nil && len(privBytes) == crypto.KeySize {
			pubBytes, _ := base64.StdEncoding.DecodeString(cfg.PublicKey)
			b.keyPair = &crypto.KeyPair{}
			copy(b.keyPair.PrivateKey[:], privBytes)
			copy(b.keyPair.PublicKey[:], pubBytes)
			log.Println("E2EE: Keys loaded")
		}
	}

	if cfg.WebPubKey != "" {
		b.webPubKey, _ = crypto.PublicKeyFromBase64(cfg.WebPubKey)
	}

	return b, nil
}

func (b *Bridge) Start() error {
	// Start permission server
	if err := b.permServer.Start(); err != nil {
		log.Printf("Warning: Could not start permission server: %v", err)
	}

	// Set up permission request forwarding
	b.permHandler.OnRequest(func(req permission.Request) {
		req.DeviceID = b.config.DeviceID
		b.sendMessage(Message{
			Type:      "permission:request",
			Payload:   req,
			Timestamp: time.Now().UnixMilli(),
		})
	})

	// Set up session output forwarding
	b.sessions.SetOutputCallback(func(sessionID, outputType, content string) {
		b.sendMessage(Message{
			Type: "session:output",
			Payload: map[string]interface{}{
				"sessionId":  sessionID,
				"deviceId":   b.config.DeviceID,
				"outputType": outputType,
				"content":    content,
			},
			Timestamp: time.Now().UnixMilli(),
		})
	})

	if err := b.connect(); err != nil {
		return err
	}

	// Send device online message
	b.sendMessage(Message{
		Type: "device:online",
		Payload: map[string]string{
			"deviceId":   b.config.DeviceID,
			"deviceName": getDeviceName(),
		},
		Timestamp: time.Now().UnixMilli(),
	})

	// Start message handler
	go b.readLoop()

	// Start heartbeat
	go b.heartbeat()

	// Wait for shutdown
	<-b.done
	return nil
}

func (b *Bridge) Stop() {
	close(b.done)
	b.sessions.StopAll()
	b.permServer.Stop()
	if b.conn != nil {
		b.conn.Close()
	}
}

func (b *Bridge) connect() error {
	u, err := url.Parse(b.config.ServerURL)
	if err != nil {
		return err
	}

	// Add connection parameters
	q := u.Query()
	q.Set("type", "bridge")
	q.Set("deviceId", b.config.DeviceID)
	q.Set("token", b.config.DeviceToken)
	u.RawQuery = q.Encode()
	u.Path = fmt.Sprintf("/ws/%s", b.config.UserID)

	log.Printf("Connecting to %s", u.String())

	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		// For demo, continue without connection
		log.Printf("Warning: Could not connect to server: %v", err)
		log.Println("Running in offline mode...")
		return nil
	}

	b.conn = conn
	log.Println("Connected to server")
	return nil
}

func (b *Bridge) readLoop() {
	if b.conn == nil {
		return
	}

	for {
		select {
		case <-b.done:
			return
		default:
			_, data, err := b.conn.ReadMessage()
			if err != nil {
				log.Printf("WebSocket read error: %v", err)
				b.reconnect()
				continue
			}

			var msg Message
			if err := json.Unmarshal(data, &msg); err != nil {
				log.Printf("Failed to parse message: %v", err)
				continue
			}

			b.handleMessage(msg)
		}
	}
}

func (b *Bridge) handleMessage(msg Message) {
	switch msg.Type {
	case "session:start":
		b.handleSessionStart(msg)
	case "session:send":
		b.handleSessionSend(msg)
	case "session:stop":
		b.handleSessionStop(msg)
	case "permission:response":
		b.handlePermissionResponse(msg)
	case "control:takeover":
		b.handleControlTakeover(msg)
	default:
		log.Printf("Unknown message type: %s", msg.Type)
	}
}

func (b *Bridge) handleSessionStart(msg Message) {
	payload, ok := msg.Payload.(map[string]interface{})
	if !ok {
		return
	}

	cliType, _ := payload["cliType"].(string)
	workDir, _ := payload["workDir"].(string)
	initialCommand, _ := payload["command"].(string)

	if cliType == "" {
		cliType = "kiro" // default
	}
	if workDir == "" {
		workDir = "."
	}

	sess, err := b.sessions.Create(cliType, workDir)
	if err != nil {
		log.Printf("Failed to create session: %v", err)
		b.sendMessage(Message{
			Type: "session:error",
			Payload: map[string]interface{}{
				"error": err.Error(),
			},
			Timestamp: time.Now().UnixMilli(),
		})
		return
	}

	// Send session started notification
	b.sendMessage(Message{
		Type: "session:started",
		Payload: map[string]interface{}{
			"sessionId": sess.ID,
			"deviceId":  b.config.DeviceID,
			"cliType":   cliType,
			"workDir":   workDir,
		},
		Timestamp: time.Now().UnixMilli(),
	})

	// Send initial command if provided
	if initialCommand != "" {
		sess.Send(initialCommand)
	}
}

func (b *Bridge) handleSessionSend(msg Message) {
	payload, ok := msg.Payload.(map[string]interface{})
	if !ok {
		return
	}

	sessionID, _ := payload["sessionId"].(string)
	content, _ := payload["content"].(string)

	sess := b.sessions.Get(sessionID)
	if sess == nil {
		log.Printf("Session not found: %s", sessionID)
		return
	}

	if err := sess.Send(content); err != nil {
		log.Printf("Send error: %v", err)
	}
}

func (b *Bridge) handleSessionStop(msg Message) {
	payload, ok := msg.Payload.(map[string]interface{})
	if !ok {
		return
	}

	sessionID, _ := payload["sessionId"].(string)
	if err := b.sessions.Stop(sessionID); err != nil {
		log.Printf("Failed to stop session: %v", err)
	}

	// Send session stopped notification
	b.sendMessage(Message{
		Type: "session:stopped",
		Payload: map[string]interface{}{
			"sessionId": sessionID,
			"deviceId":  b.config.DeviceID,
		},
		Timestamp: time.Now().UnixMilli(),
	})
}

func (b *Bridge) handlePermissionResponse(msg Message) {
	payload, ok := msg.Payload.(map[string]interface{})
	if !ok {
		log.Printf("Invalid permission response payload")
		return
	}

	id, _ := payload["id"].(string)
	approved, _ := payload["approved"].(bool)

	b.permHandler.Resolve(permission.Response{
		ID:       id,
		Approved: approved,
	})
}

func (b *Bridge) handleControlTakeover(msg Message) {
	payload, ok := msg.Payload.(map[string]interface{})
	if !ok {
		return
	}

	sessionID, _ := payload["sessionId"].(string)
	log.Printf("Control takeover for session: %s", sessionID)
	// TODO: Implement control takeover
}

func (b *Bridge) sendMessage(msg Message) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.conn == nil {
		log.Printf("Offline: %s", msg.Type)
		return nil
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	// Encrypt if E2EE is enabled and we have web's public key
	if b.keyPair != nil && b.webPubKey != nil {
		encrypted, err := b.keyPair.Encrypt(data, b.webPubKey)
		if err != nil {
			log.Printf("Encryption failed: %v", err)
			return b.conn.WriteMessage(websocket.TextMessage, data)
		}

		envelope := Message{
			Type: "encrypted",
			Payload: map[string]string{
				"data":   base64.StdEncoding.EncodeToString(encrypted),
				"pubKey": b.keyPair.PublicKeyBase64(),
			},
			Timestamp: time.Now().UnixMilli(),
		}
		data, _ = json.Marshal(envelope)
	}

	return b.conn.WriteMessage(websocket.TextMessage, data)
}

func (b *Bridge) heartbeat() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-b.done:
			return
		case <-ticker.C:
			if b.conn != nil {
				b.conn.WriteMessage(websocket.PingMessage, nil)
			}
		}
	}
}

func (b *Bridge) reconnect() {
	log.Println("Reconnecting...")
	time.Sleep(5 * time.Second)
	b.connect()
}

func getDeviceName() string {
	hostname, err := os.Hostname()
	if err != nil {
		return "Unknown Device"
	}
	return hostname
}

// Message represents a WebSocket message
type Message struct {
	Type      string      `json:"type"`
	Payload   interface{} `json:"payload"`
	Timestamp int64       `json:"timestamp"`
}
