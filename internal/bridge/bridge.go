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
	"github.com/open-agents/bridge/internal/api"
	"github.com/open-agents/bridge/internal/config"
	"github.com/open-agents/bridge/internal/crypto"
	"github.com/open-agents/bridge/internal/permission"
	"github.com/open-agents/bridge/internal/protocol"
	"github.com/open-agents/bridge/internal/rules"
	"github.com/open-agents/bridge/internal/session"
	"github.com/open-agents/bridge/internal/storage"
)

type Bridge struct {
	config       *config.Config
	conn         *websocket.Conn
	sessions     *session.Manager
	permServer   *permission.Server
	permHandler  *permission.Handler
	store        *storage.Store
	s3Uploader   *storage.S3Uploader
	rulesEngine  *rules.Engine
	apiClient    *api.Client
	keyPair      *crypto.KeyPair
	webPubKey    *[crypto.KeySize]byte
	done         chan struct{}
	mu           sync.Mutex
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
		rulesEngine: rules.NewEngine(cfg.Rules),
		apiClient:   api.NewClient(cfg),
		done:        make(chan struct{}),
	}

	// Initialize S3 uploader if configured
	if cfg.S3Config != nil {
		b.s3Uploader = storage.NewS3Uploader(cfg.S3Config)
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

	// Sync rules from API on startup
	go b.syncRulesFromAPI()

	// Set up permission request forwarding with rules engine
	b.permHandler.OnRequest(func(req permission.Request) {
		req.DeviceID = b.config.DeviceID

		// Check auto-approval rules
		path := ""
		command := ""
		if req.Detail != nil {
			if p, ok := req.Detail["path"].(string); ok {
				path = p
			}
			if c, ok := req.Detail["command"].(string); ok {
				command = c
			}
		}

		action, ruleID := b.rulesEngine.Evaluate(req.PermissionType, path, command)

		switch action {
		case "auto-approve":
			log.Printf("Auto-approved by rule %s: %s", ruleID, req.Description)
			b.permHandler.Resolve(permission.Response{ID: req.ID, Approved: true})
			return
		case "deny":
			log.Printf("Auto-denied by rule %s: %s", ruleID, req.Description)
			b.permHandler.Resolve(permission.Response{ID: req.ID, Approved: false})
			return
		}

		// Default: forward to Web for user decision
		b.sendMessage(Message{
			Type:      "permission:request",
			Payload:   req,
			Timestamp: time.Now().UnixMilli(),
		})
	})

	// Set up session output forwarding
	b.sessions.SetOutputCallback(func(sessionID string, msg protocol.Message) {
		log.Printf("[Bridge] Forwarding protocol message: sessionId=%s, type=%s", sessionID, msg.Type)
		
		// Get session to check protocol
		sess := b.sessions.Get(sessionID)
		protocolName := "unknown"
		if sess != nil {
			protocolName = sess.GetProtocolName()
		}

		switch msg.Type {
		case protocol.MessageTypeContent:
			// AI response content
			b.sendMessage(Message{
				Type: "chat:response",
				Payload: map[string]interface{}{
					"sessionId": sessionID,
					"deviceId":  b.config.DeviceID,
					"content":   msg.Content,
					"protocol":  protocolName,
				},
				Timestamp: time.Now().UnixMilli(),
			})

		case protocol.MessageTypeThought:
			// AI thinking process
			b.sendMessage(Message{
				Type: "chat:thought",
				Payload: map[string]interface{}{
					"sessionId": sessionID,
					"deviceId":  b.config.DeviceID,
					"content":   msg.Content,
					"protocol":  protocolName,
				},
				Timestamp: time.Now().UnixMilli(),
			})

		case protocol.MessageTypeToolCall:
			// Tool invocation
			b.sendMessage(Message{
				Type: "tool:call",
				Payload: map[string]interface{}{
					"sessionId": sessionID,
					"deviceId":  b.config.DeviceID,
					"toolCall":  msg.Content,
					"protocol":  protocolName,
				},
				Timestamp: time.Now().UnixMilli(),
			})

		case protocol.MessageTypePermission:
			// Permission request
			permReq := msg.Content.(protocol.PermissionRequest)
			b.sendMessage(Message{
				Type: "permission:request",
				Payload: map[string]interface{}{
					"sessionId":   sessionID,
					"deviceId":    b.config.DeviceID,
					"id":          permReq.ID,
					"toolName":    permReq.ToolName,
					"toolInput":   permReq.ToolInput,
					"description": permReq.Description,
					"risk":        permReq.Risk,
					"options":     permReq.Options,
					"protocol":    protocolName,
				},
				Timestamp: time.Now().UnixMilli(),
			})

		case protocol.MessageTypeStatus:
			// Agent status change
			b.sendMessage(Message{
				Type: "agent:status",
				Payload: map[string]interface{}{
					"sessionId": sessionID,
					"deviceId":  b.config.DeviceID,
					"status":    msg.Content,
					"protocol":  protocolName,
				},
				Timestamp: time.Now().UnixMilli(),
			})

		case protocol.MessageTypePlan:
			// Task plan
			b.sendMessage(Message{
				Type: "agent:plan",
				Payload: map[string]interface{}{
					"sessionId": sessionID,
					"deviceId":  b.config.DeviceID,
					"plan":      msg.Content,
					"protocol":  protocolName,
				},
				Timestamp: time.Now().UnixMilli(),
			})

		case protocol.MessageTypeError:
			// Error message
			b.sendMessage(Message{
				Type: "session:error",
				Payload: map[string]interface{}{
					"sessionId": sessionID,
					"deviceId":  b.config.DeviceID,
					"error":     msg.Content,
					"protocol":  protocolName,
				},
				Timestamp: time.Now().UnixMilli(),
			})

		default:
			// For PTY raw output, send as session:output
			if protocolName == "pty" {
				b.sendMessage(Message{
					Type: "session:output",
					Payload: map[string]interface{}{
						"sessionId":  sessionID,
						"deviceId":   b.config.DeviceID,
						"outputType": "stdout",
						"content":    msg.Content,
						"protocol":   protocolName,
					},
					Timestamp: time.Now().UnixMilli(),
				})
			}
		}
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
	log.Printf("[Bridge] Received message type: %s, payload: %+v", msg.Type, msg.Payload)
	switch msg.Type {
	case "session:start":
		b.handleSessionStart(msg)
	case "session:send":
		b.handleSessionSend(msg)
	case "session:stop":
		b.handleSessionStop(msg)
	case "session:cancel":
		b.handleSessionCancel(msg)
	case "session:resize":
		b.handleSessionResize(msg)
	case "chat:send":
		b.handleChatSend(msg)
	case "permission:response":
		b.handlePermissionResponse(msg)
	case "control:takeover":
		b.handleControlTakeover(msg)
	case "config:sync":
		b.handleConfigSync(msg)
	case "rules:sync":
		b.handleRulesSync(msg)
	case "storage:sync":
		b.handleStorageSync(msg)
	default:
		log.Printf("Unknown message type: %s", msg.Type)
	}
}

func (b *Bridge) handleSessionStart(msg Message) {
	log.Printf("[Bridge] handleSessionStart called")
	payload, ok := msg.Payload.(map[string]interface{})
	if !ok {
		log.Printf("[Bridge] handleSessionStart: invalid payload type")
		return
	}

	sessionID, _ := payload["sessionId"].(string)
	cliType, _ := payload["cliType"].(string)
	workDir, _ := payload["workDir"].(string)
	initialCommand, _ := payload["command"].(string)

	// Get terminal size from payload
	cols := 120 // default
	rows := 30  // default
	if c, ok := payload["cols"].(float64); ok && c > 0 {
		cols = int(c)
	}
	if r, ok := payload["rows"].(float64); ok && r > 0 {
		rows = int(r)
	}

	log.Printf("[Bridge] sessionID=%s, cliType=%s, workDir=%s, cols=%d, rows=%d", sessionID, cliType, workDir, cols, rows)

	if cliType == "" {
		cliType = "kiro" // default
	}
	if workDir == "" {
		workDir = "."
	}

	sess, err := b.sessions.CreateWithIDAndSize(cliType, workDir, sessionID, cols, rows)
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

func (b *Bridge) handleSessionCancel(msg Message) {
	payload, ok := msg.Payload.(map[string]interface{})
	if !ok {
		return
	}

	sessionID, _ := payload["sessionId"].(string)
	log.Printf("[Bridge] Cancelling session: %s", sessionID)

	// Send cancel to the session (ACP protocol)
	sess := b.sessions.Get(sessionID)
	if sess == nil {
		log.Printf("Session not found: %s", sessionID)
		return
	}

	// Send session/cancel via protocol
	if sess.Protocol != nil {
		sess.Protocol.SendMessage(protocol.Message{
			Type:    protocol.MessageTypeCancel,
			Content: "user_cancelled",
		})
	}

	// Send cancelled notification
	b.sendMessage(Message{
		Type: "session:cancelled",
		Payload: map[string]interface{}{
			"sessionId": sessionID,
			"deviceId":  b.config.DeviceID,
		},
		Timestamp: time.Now().UnixMilli(),
	})
}

func (b *Bridge) handleSessionResize(msg Message) {
	payload, ok := msg.Payload.(map[string]interface{})
	if !ok {
		return
	}

	sessionID, _ := payload["sessionId"].(string)
	cols := 80
	rows := 24

	if c, ok := payload["cols"].(float64); ok {
		cols = int(c)
	}
	if r, ok := payload["rows"].(float64); ok {
		rows = int(r)
	}

	log.Printf("[Bridge] Resizing session %s to %dx%d", sessionID, cols, rows)
	if err := b.sessions.Resize(sessionID, cols, rows); err != nil {
		log.Printf("Failed to resize session: %v", err)
	}
}

func (b *Bridge) handlePermissionResponse(msg Message) {
	payload, ok := msg.Payload.(map[string]interface{})
	if !ok {
		log.Printf("Invalid permission response payload")
		return
	}

	// ID can be string or number in JSON-RPC 2.0
	var id interface{}
	if idVal, ok := payload["id"]; ok {
		id = idVal
	}
	approved, _ := payload["approved"].(bool)
	optionID, _ := payload["optionId"].(string)

	log.Printf("[Bridge] Permission response: id=%v, approved=%v, optionId=%s", id, approved, optionID)

	// Convert ID to string for internal permission handler
	var idStr string
	switch v := id.(type) {
	case string:
		idStr = v
	case float64:
		idStr = fmt.Sprintf("%d", int(v))
	}

	// First resolve internal permission handler
	b.permHandler.Resolve(permission.Response{
		ID:       idStr,
		Approved: approved,
	})

	// Also send to ACP protocol if optionId is provided
	if optionID != "" {
		// Find session by permission ID (stored in permission handler)
		// For now, send to all active sessions
		for _, sess := range b.sessions.List() {
			if sess.Protocol != nil && sess.Protocol.GetProtocolName() == "acp" {
				log.Printf("[Bridge] Sending permission response to ACP session: %s", sess.ID)
				sess.Protocol.SendMessage(protocol.Message{
					Type: protocol.MessageTypePermission,
					Content: protocol.PermissionResponse{
						ID:       id,
						OptionID: optionID,
					},
				})
			}
		}
	}
}

func (b *Bridge) handleControlTakeover(msg Message) {
	payload, ok := msg.Payload.(map[string]interface{})
	if !ok {
		return
	}

	sessionID, _ := payload["sessionId"].(string)
	log.Printf("Control takeover for session: %s", sessionID)
}

func (b *Bridge) handleConfigSync(msg Message) {
	payload, ok := msg.Payload.(map[string]interface{})
	if !ok {
		return
	}

	// Sync environment variables
	if envVars, ok := payload["envVars"].(map[string]interface{}); ok {
		b.config.EnvVars = make(map[string]string)
		for k, v := range envVars {
			if s, ok := v.(string); ok {
				b.config.EnvVars[k] = s
			}
		}
		// Apply to current process
		for k, v := range b.config.EnvVars {
			os.Setenv(k, v)
		}
		log.Printf("Synced %d environment variables", len(b.config.EnvVars))
	}

	// Sync CLI enabled status
	if cliEnabled, ok := payload["cliEnabled"].(map[string]interface{}); ok {
		b.config.CLIEnabled = make(map[string]bool)
		for k, v := range cliEnabled {
			if bv, ok := v.(bool); ok {
				b.config.CLIEnabled[k] = bv
			}
		}
		log.Printf("Synced CLI enabled: %v", b.config.CLIEnabled)
	}

	// Sync permissions
	if perms, ok := payload["permissions"].(map[string]interface{}); ok {
		b.config.Permissions = make(map[string]bool)
		for k, v := range perms {
			if bv, ok := v.(bool); ok {
				b.config.Permissions[k] = bv
			}
		}
		log.Printf("Synced permissions: %v", b.config.Permissions)
	}

	// Save config
	if err := config.Save(b.config); err != nil {
		log.Printf("Failed to save config: %v", err)
	}

	// Send ack
	b.sendMessage(Message{
		Type:      "config:synced",
		Payload:   map[string]string{"deviceId": b.config.DeviceID},
		Timestamp: time.Now().UnixMilli(),
	})
}

func (b *Bridge) handleRulesSync(msg Message) {
	payload, ok := msg.Payload.(map[string]interface{})
	if !ok {
		return
	}

	rulesData, ok := payload["rules"].([]interface{})
	if !ok {
		return
	}

	var newRules []config.AutoApprovalRule
	for _, r := range rulesData {
		if ruleMap, ok := r.(map[string]interface{}); ok {
			rule := config.AutoApprovalRule{
				ID:      getString(ruleMap, "id"),
				Pattern: getString(ruleMap, "pattern"),
				Tool:    getString(ruleMap, "tool"),
				Action:  getString(ruleMap, "action"),
			}
			newRules = append(newRules, rule)
		}
	}

	b.config.Rules = newRules
	b.rulesEngine.UpdateRules(newRules)
	config.Save(b.config)

	log.Printf("Synced %d auto-approval rules", len(newRules))

	b.sendMessage(Message{
		Type:      "rules:synced",
		Payload:   map[string]interface{}{"deviceId": b.config.DeviceID, "count": len(newRules)},
		Timestamp: time.Now().UnixMilli(),
	})
}

func (b *Bridge) handleStorageSync(msg Message) {
	payload, ok := msg.Payload.(map[string]interface{})
	if !ok {
		return
	}

	storageType, _ := payload["storageType"].(string)
	b.config.StorageType = storageType

	if storageType == "s3" {
		if s3Data, ok := payload["s3Config"].(map[string]interface{}); ok {
			b.config.S3Config = &config.S3Config{
				Bucket:    getString(s3Data, "bucket"),
				Region:    getString(s3Data, "region"),
				AccessKey: getString(s3Data, "accessKey"),
				SecretKey: getString(s3Data, "secretKey"),
				Endpoint:  getString(s3Data, "endpoint"),
			}
			b.s3Uploader = storage.NewS3Uploader(b.config.S3Config)
		}
	}

	config.Save(b.config)
	log.Printf("Storage type set to: %s", storageType)

	b.sendMessage(Message{
		Type:      "storage:synced",
		Payload:   map[string]string{"deviceId": b.config.DeviceID, "storageType": storageType},
		Timestamp: time.Now().UnixMilli(),
	})
}

func getString(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func (b *Bridge) handleChatSend(msg Message) {
	payload, ok := msg.Payload.(map[string]interface{})
	if !ok {
		return
	}

	sessionID, _ := payload["sessionId"].(string)
	content, _ := payload["content"].(string)

	log.Printf("Chat message for session %s: %s", sessionID, content)

	sess := b.sessions.Get(sessionID)
	if sess == nil {
		var err error
		sess, err = b.sessions.Create("kiro", ".")
		if err != nil {
			log.Printf("Failed to create session: %v", err)
			return
		}
	}

	if err := sess.Send(content); err != nil {
		log.Printf("Failed to send to CLI: %v", err)
	}
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

// syncRulesFromAPI fetches permission rules from API and updates local config
func (b *Bridge) syncRulesFromAPI() {
	rules, err := b.apiClient.GetPermissionRules("")
	if err != nil {
		log.Printf("Failed to sync rules from API: %v", err)
		return
	}

	var configRules []config.AutoApprovalRule
	for _, r := range rules {
		configRules = append(configRules, config.AutoApprovalRule{
			ID:      r.ID,
			Pattern: r.Pattern,
			Tool:    r.Tool,
			Action:  r.Action,
		})
	}

	b.config.Rules = configRules
	b.rulesEngine.UpdateRules(configRules)
	config.Save(b.config)
	log.Printf("Synced %d rules from API", len(configRules))
}

// ReportSessionToAPI reports session status to API
func (b *Bridge) ReportSessionToAPI(sessionID, cliType, workDir, status string) {
	err := b.apiClient.ReportSession(api.SessionReport{
		SessionID: sessionID,
		CLIType:   cliType,
		WorkDir:   workDir,
		Status:    status,
	})
	if err != nil {
		log.Printf("Failed to report session to API: %v", err)
	}
}

// Message represents a WebSocket message
type Message struct {
	Type      string      `json:"type"`
	Payload   interface{} `json:"payload"`
	Timestamp int64       `json:"timestamp"`
}
