package meeting

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"realtime-caption-translator/internal/database"
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

// EndMeeting closes a meeting, saves transcript snapshots, and disconnects participants.
func (rm *RoomManager) EndMeeting(meetingID string) error {
	rm.mu.Lock()
	room, exists := rm.activeRooms[meetingID]
	if !exists {
		rm.mu.Unlock()
		return nil
	}

	transcriptSnapshots := make(map[string]string)
	for _, lang := range room.GetTranscriptLanguages() {
		entries := room.GetTranscript(lang)
		if len(entries) == 0 {
			continue
		}
		transcriptSnapshots[lang] = formatTranscriptEntries(entries)
	}

	participants := make([]*Participant, 0, len(room.Participants))
	for _, participant := range room.Participants {
		participants = append(participants, participant)
	}

	delete(rm.activeRooms, meetingID)
	rm.mu.Unlock()

	if err := database.EndMeeting(meetingID); err != nil {
		return err
	}

	for lang, transcript := range transcriptSnapshots {
		if err := database.SaveMeetingTranscriptSnapshot(meetingID, lang, transcript); err != nil {
			log.Printf("Failed to save meeting transcript snapshot %s/%s: %v", meetingID, lang, err)
		}
	}

	message := Message{
		Type:      "meeting_ended",
		Timestamp: time.Now(),
	}
	payload, err := json.Marshal(message)
	if err != nil {
		return nil
	}

	for _, participant := range participants {
		if participant.Connection != nil {
			_ = participant.Connection.WriteMessage(websocket.TextMessage, payload)
			participant.Connection.Close()
		}
	}

	return nil
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

// UpdateParticipantLanguage updates a participant's target language in a room
func (rm *RoomManager) UpdateParticipantLanguage(meetingID string, participantID int, targetLanguage string) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	room, exists := rm.activeRooms[meetingID]
	if !exists {
		return
	}

	participant, exists := room.Participants[participantID]
	if !exists {
		return
	}

	participant.TargetLanguage = targetLanguage

	// Rebuild target languages cache
	room.targetLangs = make(map[string]bool)
	for _, p := range room.Participants {
		room.targetLangs[p.TargetLanguage] = true
	}
}

// GetParticipantDiarizationSettings returns diarization settings for a participant.
func (rm *RoomManager) GetParticipantDiarizationSettings(meetingID string, participantID int) (int, int, float64) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	room, exists := rm.activeRooms[meetingID]
	if !exists {
		return 0, 0, 0
	}

	participant, exists := room.Participants[participantID]
	if !exists {
		return 0, 0, 0
	}

	return participant.MinSpeakers, participant.MaxSpeakers, participant.Strictness
}

// RemoveParticipant removes a participant from a room
func (rm *RoomManager) RemoveParticipant(meetingID string, participantID int) {
	rm.mu.Lock()

	room, exists := rm.activeRooms[meetingID]
	if !exists {
		rm.mu.Unlock()
		return
	}

	room.RemoveParticipant(participantID)
	log.Printf("Participant %d left meeting %s (remaining: %d)",
		participantID, meetingID, len(room.Participants))

	// Cleanup empty rooms
	if room.IsEmpty() {
		transcriptSnapshots := make(map[string]string)
		for _, lang := range room.GetTranscriptLanguages() {
			entries := room.GetTranscript(lang)
			if len(entries) == 0 {
				continue
			}
			transcriptSnapshots[lang] = formatTranscriptEntries(entries)
		}
		delete(rm.activeRooms, meetingID)
		log.Printf("Meeting room %s is empty - removed", meetingID)
		rm.mu.Unlock()

		clearSpeakerProfile(meetingID, participantID)

		if err := database.EndMeeting(meetingID); err != nil {
			log.Printf("Failed to mark meeting ended %s: %v", meetingID, err)
		}

		for lang, transcript := range transcriptSnapshots {
			if err := database.SaveMeetingTranscriptSnapshot(meetingID, lang, transcript); err != nil {
				log.Printf("Failed to save meeting transcript snapshot %s/%s: %v", meetingID, lang, err)
			}
		}
		return
	}

	rm.mu.Unlock()

	clearSpeakerProfile(meetingID, participantID)
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

	if message.Type == "transcription" {
		room.AddTranscriptFromMessage(message)
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

// GetTranscript retrieves the transcript for a meeting and language
func (rm *RoomManager) GetTranscript(meetingID, language string) []TranscriptEntry {
	rm.mu.RLock()
	room, exists := rm.activeRooms[meetingID]
	rm.mu.RUnlock()
	if !exists {
		return nil
	}
	return room.GetTranscript(language)
}

// GetTranscriptLanguages returns all transcript languages for a meeting
func (rm *RoomManager) GetTranscriptLanguages(meetingID string) []string {
	rm.mu.RLock()
	room, exists := rm.activeRooms[meetingID]
	rm.mu.RUnlock()
	if !exists {
		return nil
	}
	return room.GetTranscriptLanguages()
}

// GetActiveRoomCount returns the number of active rooms
func (rm *RoomManager) GetActiveRoomCount() int {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return len(rm.activeRooms)
}

func formatTranscriptEntries(entries []TranscriptEntry) string {
	var b strings.Builder
	for _, entry := range entries {
		speaker := entry.SpeakerName
		if speaker == "" {
			speaker = entry.SpeakerID
		}
		if speaker == "" {
			speaker = "Speaker"
		}
		ts := entry.Timestamp.Format("15:04:05")
		b.WriteString(fmt.Sprintf("[%s] %s: %s\n", ts, speaker, entry.Text))
	}
	return b.String()
}
