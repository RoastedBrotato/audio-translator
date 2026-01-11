package main

import (
	"flag"
	"log"
	"os"
	"time"

	"realtime-caption-translator/internal/database"
	"realtime-caption-translator/internal/llm"
	"realtime-caption-translator/internal/meeting"
)

func main() {
	limit := flag.Int("limit", 25, "Maximum number of meetings to backfill")
	language := flag.String("language", "en", "Transcript language to backfill")
	llmURL := flag.String("llm-url", "", "LLM service base URL (default http://127.0.0.1:8007)")
	flag.Parse()

	if *llmURL == "" {
		*llmURL = getEnv("LLM_URL", "http://127.0.0.1:8007")
	}

	if err := database.Init(); err != nil {
		log.Fatalf("Database init failed: %v", err)
	}
	defer database.Close()

	llmClient := llm.New(*llmURL)

	meetingIDs, err := listMeetingsMissingMinutes(*language, *limit)
	if err != nil {
		log.Fatalf("Failed to list meetings: %v", err)
	}

	if len(meetingIDs) == 0 {
		log.Println("No meetings require minutes backfill.")
		return
	}

	log.Printf("Backfilling minutes for %d meetings (language: %s)", len(meetingIDs), *language)
	for _, meetingID := range meetingIDs {
		log.Printf("Generating minutes for %s", meetingID)
		if err := meeting.GenerateMeetingMinutes(meetingID, *language, llmClient); err != nil {
			log.Printf("Minutes failed for %s: %v", meetingID, err)
			continue
		}
		log.Printf("Minutes stored for %s", meetingID)
	}
}

func listMeetingsMissingMinutes(language string, limit int) ([]string, error) {
	if limit <= 0 {
		limit = 25
	}
	query := `
		SELECT s.meeting_id, MAX(s.created_at) AS latest_snapshot
		FROM meeting_transcript_snapshots s
		LEFT JOIN meeting_minutes mm ON mm.meeting_id = s.meeting_id AND mm.language = $1
		WHERE s.language = $1 AND mm.id IS NULL
		GROUP BY s.meeting_id
		ORDER BY latest_snapshot DESC
		LIMIT $2
	`

	rows, err := database.DB.Query(query, language, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var meetingIDs []string
	for rows.Next() {
		var meetingID string
		var latestSnapshot time.Time
		if err := rows.Scan(&meetingID, &latestSnapshot); err != nil {
			return nil, err
		}
		meetingIDs = append(meetingIDs, meetingID)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return meetingIDs, nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
