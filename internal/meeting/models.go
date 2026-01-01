package meeting

import (
	"fmt"
	"sync"
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
	SpeakerID            string            `json:"speakerId,omitempty"` // For speaker diarization (e.g., "SPEAKER_00")
	SpeakerName          string            `json:"speakerName,omitempty"`
	OriginalText         string            `json:"originalText,omitempty"`
	SourceLanguage       string            `json:"sourceLanguage,omitempty"`
	Translations         map[string]string `json:"translations,omitempty"`
	IsFinal              bool              `json:"isFinal,omitempty"`
	Timestamp            time.Time         `json:"timestamp"`
	Error                string            `json:"error,omitempty"`
}

// TranscriptEntry represents one line in a language-specific transcript
type TranscriptEntry struct {
	Timestamp   time.Time `json:"timestamp"`
	SpeakerID   string    `json:"speakerId,omitempty"`
	SpeakerName string    `json:"speakerName,omitempty"`
	Text        string    `json:"text"`
}

// Room represents an active meeting room
type Room struct {
	MeetingID    string
	Participants map[int]*Participant // participantId -> Participant
	targetLangs  map[string]bool      // Cache of unique target languages

	// Audio mixing for shared room mode
	audioBuffers map[int][]int16 // participantId -> audio samples
	audioMutex   sync.RWMutex    // Protect concurrent access to audioBuffers

	// Speaker mapping for consistency
	speakerMap    map[int]string // participantId -> assigned SPEAKER_XX (for persistence)
	nextSpeakerID int            // Counter for assigning SPEAKER_IDs

	// Transcript storage (per language)
	transcriptMu sync.RWMutex
	transcripts  map[string][]TranscriptEntry // language -> entries
}

// NewRoom creates a new room
func NewRoom(meetingID string) *Room {
	return &Room{
		MeetingID:     meetingID,
		Participants:  make(map[int]*Participant),
		targetLangs:   make(map[string]bool),
		audioBuffers:  make(map[int][]int16),
		speakerMap:    make(map[int]string),
		nextSpeakerID: 0,
		transcripts:   make(map[string][]TranscriptEntry),
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

// AddAudioBuffer adds audio samples from a participant
func (r *Room) AddAudioBuffer(participantID int, samples []int16) {
	r.audioMutex.Lock()
	defer r.audioMutex.Unlock()

	r.audioBuffers[participantID] = append(r.audioBuffers[participantID], samples...)
}

// GetMixedAudio combines audio from all participants and returns it
// This simulates what happens when multiple people speak in the same room
func (r *Room) GetMixedAudio(maxSamples int) []int16 {
	r.audioMutex.RLock()
	defer r.audioMutex.RUnlock()

	if len(r.audioBuffers) == 0 {
		return []int16{}
	}

	// Find the maximum length
	maxLen := 0
	for _, samples := range r.audioBuffers {
		if len(samples) > maxLen {
			maxLen = len(samples)
		}
	}

	// Limit to maxSamples if specified
	if maxSamples > 0 && maxLen > maxSamples {
		maxLen = maxSamples
	}

	// Mix audio by averaging
	mixed := make([]int16, maxLen)
	participantCount := len(r.audioBuffers)

	for participantID, samples := range r.audioBuffers {
		// Ensure this participant is mapped to a speaker ID
		if _, exists := r.speakerMap[participantID]; !exists {
			r.speakerMap[participantID] = formatSpeakerIDFromInt(r.nextSpeakerID)
			r.nextSpeakerID++
		}

		for i := 0; i < len(samples) && i < maxLen; i++ {
			// Simple average mixing (normalize to prevent clipping)
			val := int32(samples[i]) / int32(participantCount)
			mixed[i] += int16(val)
		}
	}

	return mixed
}

// ClearAudioBuffers clears all audio buffers (call after processing)
func (r *Room) ClearAudioBuffers() {
	r.audioMutex.Lock()
	defer r.audioMutex.Unlock()
	r.audioBuffers = make(map[int][]int16)
}

// GetSpeakerIDForParticipant returns the assigned SPEAKER_XX for a participant
func (r *Room) GetSpeakerIDForParticipant(participantID int) string {
	r.audioMutex.RLock()
	defer r.audioMutex.RUnlock()

	speakerID, exists := r.speakerMap[participantID]
	if exists {
		return speakerID
	}
	return ""
}

// formatSpeakerIDFromInt converts an integer to SPEAKER_XX format
func formatSpeakerIDFromInt(num int) string {
	return "SPEAKER_" + fmt.Sprintf("%02d", num)
}

// AddTranscriptFromMessage stores a transcription message for later download
func (r *Room) AddTranscriptFromMessage(message Message) {
	if message.Type != "transcription" || message.OriginalText == "" {
		return
	}

	r.transcriptMu.Lock()
	defer r.transcriptMu.Unlock()

	if r.transcripts == nil {
		r.transcripts = make(map[string][]TranscriptEntry)
	}

	if len(message.Translations) > 0 {
		for lang, translated := range message.Translations {
			text := translated
			if text == "" {
				text = message.OriginalText
			}
			r.transcripts[lang] = append(r.transcripts[lang], TranscriptEntry{
				Timestamp:   message.Timestamp,
				SpeakerID:   message.SpeakerID,
				SpeakerName: message.SpeakerName,
				Text:        text,
			})
		}
		if message.SourceLanguage != "" {
			if _, exists := message.Translations[message.SourceLanguage]; !exists {
				r.transcripts[message.SourceLanguage] = append(r.transcripts[message.SourceLanguage], TranscriptEntry{
					Timestamp:   message.Timestamp,
					SpeakerID:   message.SpeakerID,
					SpeakerName: message.SpeakerName,
					Text:        message.OriginalText,
				})
			}
		}
		return
	}

	lang := message.SourceLanguage
	if lang == "" {
		lang = "und"
	}
	r.transcripts[lang] = append(r.transcripts[lang], TranscriptEntry{
		Timestamp:   message.Timestamp,
		SpeakerID:   message.SpeakerID,
		SpeakerName: message.SpeakerName,
		Text:        message.OriginalText,
	})
}

// GetTranscript returns the transcript for a specific language
func (r *Room) GetTranscript(language string) []TranscriptEntry {
	r.transcriptMu.RLock()
	defer r.transcriptMu.RUnlock()

	if r.transcripts == nil {
		return nil
	}
	entries := r.transcripts[language]
	out := make([]TranscriptEntry, len(entries))
	copy(out, entries)
	return out
}

// GetTranscriptLanguages returns the languages that have transcript entries
func (r *Room) GetTranscriptLanguages() []string {
	r.transcriptMu.RLock()
	defer r.transcriptMu.RUnlock()

	if r.transcripts == nil {
		return nil
	}
	langs := make([]string, 0, len(r.transcripts))
	for lang := range r.transcripts {
		langs = append(langs, lang)
	}
	return langs
}
