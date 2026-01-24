package test

import (
	"testing"
	"time"

	"github.com/open-agents/bridge/internal/permission"
)

func TestPermissionHandler(t *testing.T) {
	handler := permission.NewHandler()

	var receivedReq permission.Request
	handler.OnRequest(func(req permission.Request) {
		receivedReq = req
	})

	req := permission.Request{
		ID:             "perm_1",
		SessionID:      "session_1",
		DeviceID:       "device_1",
		PermissionType: "file:write",
		Description:    "Write to test.txt",
		Risk:           "low",
		Timeout:        2,
	}

	// Submit in goroutine
	done := make(chan bool)
	var approved bool
	go func() {
		approved, _ = handler.Submit(req)
		done <- true
	}()

	// Wait for request to be received
	time.Sleep(50 * time.Millisecond)

	if receivedReq.ID != req.ID {
		t.Errorf("Received request ID = %s, want %s", receivedReq.ID, req.ID)
	}

	// Resolve the request
	handler.Resolve(permission.Response{ID: req.ID, Approved: true})

	<-done

	if !approved {
		t.Error("Expected request to be approved")
	}
}

func TestPermissionHandlerTimeout(t *testing.T) {
	handler := permission.NewHandler()

	req := permission.Request{
		ID:      "perm_timeout",
		Timeout: 1, // 1 second timeout
	}

	start := time.Now()
	approved, _ := handler.Submit(req)
	elapsed := time.Since(start)

	if approved {
		t.Error("Expected timeout to deny request")
	}

	if elapsed < time.Second {
		t.Error("Should have waited for timeout")
	}
}

func TestPermissionHandlerResolveUnknown(t *testing.T) {
	handler := permission.NewHandler()

	// Should not panic
	handler.Resolve(permission.Response{ID: "unknown", Approved: true})
}
