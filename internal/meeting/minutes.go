package meeting

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"realtime-caption-translator/internal/database"
	"realtime-caption-translator/internal/llm"
)

// GenerateMeetingMinutes builds and stores meeting minutes for a meeting/language.
func GenerateMeetingMinutes(meetingID, language string, llmClient *llm.Client) error {
	if llmClient == nil {
		return fmt.Errorf("llm client is nil")
	}
	if language == "" {
		language = "en"
	}

	snapshot, err := database.GetMeetingTranscriptSnapshot(meetingID, language)
	if err != nil {
		return fmt.Errorf("failed to load transcript snapshot: %w", err)
	}
	if snapshot == nil || strings.TrimSpace(snapshot.Transcript) == "" {
		return fmt.Errorf("empty transcript snapshot")
	}

	participants, err := database.GetMeetingParticipants(meetingID)
	if err != nil {
		log.Printf("Failed to load participants for minutes: %v", err)
	}

	participantNames := make([]string, 0, len(participants))
	seen := make(map[string]struct{})
	for _, participant := range participants {
		name := strings.TrimSpace(participant.ParticipantName)
		if name == "" {
			continue
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		participantNames = append(participantNames, name)
	}

	context := snapshot.Transcript
	const maxContextChars = 12000
	if len(context) > maxContextChars {
		context = context[:maxContextChars] + "\n[Transcript truncated]"
	}

	prompt := "Create meeting minutes as JSON with keys: participants (array of names), key_points (array), action_items (array), decisions (array), summary (string)."
	if len(participantNames) > 0 {
		prompt += fmt.Sprintf(" Use these participants if relevant: %s.", strings.Join(participantNames, ", "))
	}
	prompt += " Return JSON only."

	answer, err := llmClient.Generate(prompt, context, 700, 0.3)
	if err != nil {
		return fmt.Errorf("minutes generation failed: %w", err)
	}

	content, err := parseMeetingMinutesJSON(answer)
	if err != nil {
		log.Printf("Minutes JSON parse failed for meeting %s: %v", meetingID, err)
		content = database.MeetingMinutesContent{
			Participants: participantNames,
			Summary:      strings.TrimSpace(answer),
		}
	}

	if len(content.Participants) == 0 && len(participantNames) > 0 {
		content.Participants = participantNames
	}
	if strings.TrimSpace(content.Summary) == "" {
		content.Summary = strings.TrimSpace(answer)
	}

	if err := database.SaveMeetingMinutes(meetingID, language, content); err != nil {
		return fmt.Errorf("failed to save meeting minutes: %w", err)
	}

	return nil
}

func parseMeetingMinutesJSON(raw string) (database.MeetingMinutesContent, error) {
	cleaned := strings.TrimSpace(raw)
	if strings.HasPrefix(cleaned, "```") {
		cleaned = strings.TrimPrefix(cleaned, "```json")
		cleaned = strings.TrimPrefix(cleaned, "```")
		cleaned = strings.TrimSuffix(cleaned, "```")
		cleaned = strings.TrimSpace(cleaned)
	}

	start := strings.Index(cleaned, "{")
	end := strings.LastIndex(cleaned, "}")
	if start == -1 || end == -1 || end <= start {
		return database.MeetingMinutesContent{}, fmt.Errorf("no JSON object found")
	}

	cleaned = cleaned[start : end+1]

	var content database.MeetingMinutesContent
	if err := json.Unmarshal([]byte(cleaned), &content); err != nil {
		return database.MeetingMinutesContent{}, err
	}

	return content, nil
}
