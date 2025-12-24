package progress

import (
	"encoding/json"
	"log"
	"sync"

	"github.com/gorilla/websocket"
)

// Update represents a progress update message
type Update struct {
	SessionID string                 `json:"sessionId"`
	Stage     string                 `json:"stage"`
	Progress  float64                `json:"progress"` // 0-100
	Message   string                 `json:"message"`
	Error     string                 `json:"error,omitempty"`
	Results   map[string]interface{} `json:"results,omitempty"`
}

// Tracker tracks progress for a single upload session
type Tracker struct {
	SessionID string
	manager   *Manager
}

// Manager manages progress tracking for multiple upload sessions
type Manager struct {
	mu          sync.RWMutex
	subscribers map[string][]*websocket.Conn
}

// NewManager creates a new progress manager
func NewManager() *Manager {
	return &Manager{
		subscribers: make(map[string][]*websocket.Conn),
	}
}

// Subscribe adds a WebSocket connection to receive progress updates for a session
func (m *Manager) Subscribe(sessionID string, conn *websocket.Conn) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.subscribers[sessionID] == nil {
		m.subscribers[sessionID] = make([]*websocket.Conn, 0)
	}
	m.subscribers[sessionID] = append(m.subscribers[sessionID], conn)
	log.Printf("Progress subscriber added for session %s (total: %d)", sessionID, len(m.subscribers[sessionID]))
}

// Unsubscribe removes a WebSocket connection from receiving updates
func (m *Manager) Unsubscribe(sessionID string, conn *websocket.Conn) {
	m.mu.Lock()
	defer m.mu.Unlock()

	subscribers := m.subscribers[sessionID]
	for i, sub := range subscribers {
		if sub == conn {
			m.subscribers[sessionID] = append(subscribers[:i], subscribers[i+1:]...)
			log.Printf("Progress subscriber removed for session %s", sessionID)
			break
		}
	}

	// Cleanup if no more subscribers
	if len(m.subscribers[sessionID]) == 0 {
		delete(m.subscribers, sessionID)
	}
}

// SendUpdate sends a progress update to all subscribers of a session
func (m *Manager) SendUpdate(update Update) {
	m.mu.RLock()
	subscribers := m.subscribers[update.SessionID]
	m.mu.RUnlock()

	if len(subscribers) == 0 {
		return
	}

	data, err := json.Marshal(update)
	if err != nil {
		log.Printf("Error marshaling progress update: %v", err)
		return
	}

	// Send to all subscribers (create copy to avoid holding lock)
	m.mu.RLock()
	subs := make([]*websocket.Conn, len(subscribers))
	copy(subs, subscribers)
	m.mu.RUnlock()

	for _, conn := range subs {
		if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
			log.Printf("Error sending progress update: %v", err)
			// Remove failed connection
			m.Unsubscribe(update.SessionID, conn)
		}
	}
}

// NewTracker creates a progress tracker for a session
func (m *Manager) NewTracker(sessionID string) *Tracker {
	return &Tracker{
		SessionID: sessionID,
		manager:   m,
	}
}

// Update sends a progress update through the manager
func (t *Tracker) Update(stage string, progress float64, message string) {
	t.manager.SendUpdate(Update{
		SessionID: t.SessionID,
		Stage:     stage,
		Progress:  progress,
		Message:   message,
	})
}

// Error sends an error update
func (t *Tracker) Error(stage string, message string, err error) {
	errMsg := ""
	if err != nil {
		errMsg = err.Error()
	}
	t.manager.SendUpdate(Update{
		SessionID: t.SessionID,
		Stage:     stage,
		Progress:  0,
		Message:   message,
		Error:     errMsg,
	})
}

// Complete sends a completion update
func (t *Tracker) Complete(message string) {
	t.manager.SendUpdate(Update{
		SessionID: t.SessionID,
		Stage:     "complete",
		Progress:  100,
		Message:   message,
	})
}

// CompleteWithResults sends a completion update with result data
func (t *Tracker) CompleteWithResults(message string, results map[string]interface{}) {
	t.manager.SendUpdate(Update{
		SessionID: t.SessionID,
		Stage:     "complete",
		Progress:  100,
		Message:   message,
		Results:   results,
	})
}
