package meeting

import (
	"time"

	"github.com/gorilla/websocket"
)

// Participant represents an active participant in a meeting room
type Participant struct {
	ID             int
	Name           string
	TargetLanguage string
	JoinedAt       time.Time
	Connection     *websocket.Conn
}

// Message represents a message to be broadcast to meeting participants
type Message struct {
	Type                 string            `json:"type"`
	ParticipantID        int               `json:"participantId,omitempty"`
	ParticipantName      string            `json:"participantName,omitempty"`
	TargetLanguage       string            `json:"targetLanguage,omitempty"`
	SpeakerParticipantID int               `json:"speakerParticipantId,omitempty"`
	SpeakerName          string            `json:"speakerName,omitempty"`
	OriginalText         string            `json:"originalText,omitempty"`
	SourceLanguage       string            `json:"sourceLanguage,omitempty"`
	Translations         map[string]string `json:"translations,omitempty"`
	IsFinal              bool              `json:"isFinal,omitempty"`
	Timestamp            time.Time         `json:"timestamp"`
	Error                string            `json:"error,omitempty"`
}

// Room represents an active meeting room
type Room struct {
	MeetingID    string
	Participants map[int]*Participant // participantId -> Participant
	targetLangs  map[string]bool      // Cache of unique target languages
}

// NewRoom creates a new room
func NewRoom(meetingID string) *Room {
	return &Room{
		MeetingID:    meetingID,
		Participants: make(map[int]*Participant),
		targetLangs:  make(map[string]bool),
	}
}

// AddParticipant adds a participant to the room
func (r *Room) AddParticipant(p *Participant) {
	r.Participants[p.ID] = p
	r.targetLangs[p.TargetLanguage] = true
}

// RemoveParticipant removes a participant from the room
func (r *Room) RemoveParticipant(participantID int) {
	delete(r.Participants, participantID)

	// Rebuild target languages cache
	r.targetLangs = make(map[string]bool)
	for _, p := range r.Participants {
		r.targetLangs[p.TargetLanguage] = true
	}
}

// GetUniqueTargetLanguages returns all unique target languages in the room
func (r *Room) GetUniqueTargetLanguages() []string {
	languages := make([]string, 0, len(r.targetLangs))
	for lang := range r.targetLangs {
		languages = append(languages, lang)
	}
	return languages
}

// IsEmpty returns true if the room has no participants
func (r *Room) IsEmpty() bool {
	return len(r.Participants) == 0
}
