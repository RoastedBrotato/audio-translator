package meeting

import (
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// RoomManager manages active meeting rooms
// Pattern based on progress.Manager for WebSocket broadcasting
type RoomManager struct {
	mu          sync.RWMutex
	activeRooms map[string]*Room // meetingId -> Room
}

// NewRoomManager creates a new room manager
func NewRoomManager() *RoomManager {
	return &RoomManager{
		activeRooms: make(map[string]*Room),
	}
}

// GetOrCreateRoom gets an existing room or creates a new one
func (rm *RoomManager) GetOrCreateRoom(meetingID string) *Room {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	room, exists := rm.activeRooms[meetingID]
	if !exists {
		room = NewRoom(meetingID)
		rm.activeRooms[meetingID] = room
		log.Printf("Created new meeting room: %s", meetingID)
	}

	return room
}

// GetRoom gets an existing room (returns nil if doesn't exist)
func (rm *RoomManager) GetRoom(meetingID string) *Room {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return rm.activeRooms[meetingID]
}

// AddParticipant adds a participant to a room
func (rm *RoomManager) AddParticipant(meetingID string, participant *Participant) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	room, exists := rm.activeRooms[meetingID]
	if !exists {
		room = NewRoom(meetingID)
		rm.activeRooms[meetingID] = room
	}

	room.AddParticipant(participant)
	log.Printf("Participant %d (%s) joined meeting %s (total: %d)",
		participant.ID, participant.Name, meetingID, len(room.Participants))
}

// RemoveParticipant removes a participant from a room
func (rm *RoomManager) RemoveParticipant(meetingID string, participantID int) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	room, exists := rm.activeRooms[meetingID]
	if !exists {
		return
	}

	room.RemoveParticipant(participantID)
	log.Printf("Participant %d left meeting %s (remaining: %d)",
		participantID, meetingID, len(room.Participants))

	// Cleanup empty rooms
	if room.IsEmpty() {
		delete(rm.activeRooms, meetingID)
		log.Printf("Meeting room %s is empty - removed", meetingID)
	}
}

// Broadcast sends a message to all participants in a room
// Pattern from progress.Manager - thread-safe broadcasting
func (rm *RoomManager) Broadcast(meetingID string, message Message) {
	// Add timestamp
	message.Timestamp = time.Now()

	rm.mu.RLock()
	room, exists := rm.activeRooms[meetingID]
	rm.mu.RUnlock()

	if !exists || room.IsEmpty() {
		return
	}

	data, err := json.Marshal(message)
	if err != nil {
		log.Printf("Error marshaling meeting message: %v", err)
		return
	}

	// Create a copy of participants to avoid holding lock during send
	rm.mu.RLock()
	participants := make([]*Participant, 0, len(room.Participants))
	for _, p := range room.Participants {
		participants = append(participants, p)
	}
	rm.mu.RUnlock()

	// Broadcast to all participants
	for _, participant := range participants {
		if participant.Connection == nil {
			continue
		}

		if err := participant.Connection.WriteMessage(websocket.TextMessage, data); err != nil {
			log.Printf("Error sending message to participant %d: %v", participant.ID, err)
			// Note: Connection cleanup should be handled by the WebSocket handler
		}
	}
}

// GetRoomParticipants returns all participants in a room
func (rm *RoomManager) GetRoomParticipants(meetingID string) []Participant {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	room, exists := rm.activeRooms[meetingID]
	if !exists {
		return []Participant{}
	}

	participants := make([]Participant, 0, len(room.Participants))
	for _, p := range room.Participants {
		// Create a copy without the connection
		participants = append(participants, Participant{
			ID:             p.ID,
			Name:           p.Name,
			TargetLanguage: p.TargetLanguage,
			JoinedAt:       p.JoinedAt,
		})
	}

	return participants
}

// GetUniqueTargetLanguages gets all unique target languages in a room
func (rm *RoomManager) GetUniqueTargetLanguages(meetingID string) []string {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	room, exists := rm.activeRooms[meetingID]
	if !exists {
		return []string{}
	}

	return room.GetUniqueTargetLanguages()
}

// GetActiveRoomCount returns the number of active rooms
func (rm *RoomManager) GetActiveRoomCount() int {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return len(rm.activeRooms)
}
