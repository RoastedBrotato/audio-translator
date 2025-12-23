package session

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"realtime-caption-translator/internal/asr"
	"realtime-caption-translator/internal/audio"
	"realtime-caption-translator/internal/translate"
)

type Config struct {
	ASRBaseURL       string
	TranslateBaseURL string
	PollInterval     time.Duration
	WindowSeconds    int
	FinalizeAfter    time.Duration
}

type Server struct {
	cfg Config
	asr *asr.Client
	tr  translate.Translator
}

func NewServer(cfg Config) *Server {
	translator := &translate.HTTPTranslator{
		BaseURL: cfg.TranslateBaseURL,
	}
	return &Server{
		cfg: cfg,
		asr: asr.New(cfg.ASRBaseURL),
		tr:  translator,
	}
}

type controlMsg struct {
	Type       string `json:"type"`
	TargetLang string `json:"targetLang"`
	SourceLang string `json:"sourceLang"`
	SampleRate int    `json:"sampleRate"`
}

type wsEvent struct {
	Type string `json:"type"`
	ID   int    `json:"id,omitempty"`
	Text string `json:"text,omitempty"`
}

func (s *Server) HandleConn(conn *websocket.Conn) {
	defer func() {
		if r := recover(); r != nil {
			// Log panic and close gracefully
			_ = conn.WriteJSON(wsEvent{Type: "info", Text: "server error"})
		}
		conn.Close()
	}()

	var (
		targetLang = "en"
		sourceLang = ""
		sampleRate = 16000
		ring       = audio.NewRing(sampleRate * s.cfg.WindowSeconds) // samples
		started    = false

		mu          sync.Mutex
		lastPartial string
		stableSince = time.Time{}
		nextID      = 1
	)

	sendJSON := func(v any) {
		log.Printf("Sending to client: %+v", v)
		_ = conn.WriteJSON(v)
	}

	sendJSON(wsEvent{Type: "info", Text: "connected"})

	// Poll loop: ask ASR for rolling window transcript
	stopPoll := make(chan struct{})
	go func() {
		t := time.NewTicker(s.cfg.PollInterval)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				if !started {
					continue
				}
				// read last N seconds
				pcm := ring.ReadLast(sampleRate * s.cfg.WindowSeconds)
				if len(pcm) < sampleRate { // too little
					continue
				}

				// Calculate audio level
				var sum float64
				for _, sample := range pcm {
					sum += float64(sample * sample)
				}
				rms := sum / float64(len(pcm))
				log.Printf("Transcribing %d samples (%.1fs), RMS level: %.0f", len(pcm), float64(len(pcm))/float64(sampleRate), rms)

				text, err := s.asr.TranscribePCM16WithLang(pcm, sampleRate, sourceLang)
				if err != nil {
					sendJSON(wsEvent{Type: "info", Text: "ASR error: " + err.Error()})
					continue
				}
				text = strings.TrimSpace(text)
				log.Printf("ASR result: '%s'", text)

				mu.Lock()

				// Emit partial (source)
				if text != "" {
					sendJSON(wsEvent{Type: "partial", Text: text})

					// 🔹 OPTION A: translate partial immediately
					trText, err := s.tr.Translate(text, targetLang)
					if err == nil {
						sendJSON(wsEvent{
							Type: "partial_translation",
							Text: trText,
						})
					}
				} else {
					sendJSON(wsEvent{Type: "partial", Text: ""})
					sendJSON(wsEvent{Type: "partial_translation", Text: ""})
				}

				// Decide stability/finalization
				now := time.Now()
				if text == "" {
					// if we had stable partial and now silence, finalize it
					if lastPartial != "" {
						finalText := lastPartial
						id := nextID
						nextID++
						lastPartial = ""
						stableSince = time.Time{}
						mu.Unlock()

						sendJSON(wsEvent{Type: "final", ID: id, Text: finalText})
						tr, _ := s.tr.Translate(finalText, targetLang)
						sendJSON(wsEvent{Type: "translation", ID: id, Text: tr})

						// Clear ring buffer to avoid re-transcribing finalized audio
						ring.Clear()
					} else {
						mu.Unlock()
					}
					continue
				}

				if text != lastPartial {
					lastPartial = text
					stableSince = now
					mu.Unlock()
					continue
				}

				// unchanged text
				if !stableSince.IsZero() && now.Sub(stableSince) >= s.cfg.FinalizeAfter {
					finalText := lastPartial
					id := nextID
					nextID++
					lastPartial = ""
					stableSince = time.Time{}
					mu.Unlock()

					sendJSON(wsEvent{Type: "final", ID: id, Text: finalText})
					tr, _ := s.tr.Translate(finalText, targetLang)
					sendJSON(wsEvent{Type: "translation", ID: id, Text: tr})

					// Clear ring buffer to avoid re-transcribing finalized audio
					ring.Clear()
				} else {
					mu.Unlock()
				}
			case <-stopPoll:
				return
			}
		}
	}()

	// Read loop: control JSON + binary PCM frames
	for {
		mt, data, err := conn.ReadMessage()
		if err != nil {
			close(stopPoll)
			return
		}

		if mt == websocket.TextMessage {
			var msg controlMsg
			if err := json.Unmarshal(data, &msg); err != nil {
				continue
			}
			switch msg.Type {
			case "start":
				started = true
				if msg.TargetLang != "" {
					targetLang = msg.TargetLang
				}
				if msg.SourceLang != "" {
					sourceLang = msg.SourceLang
				}
				if msg.SampleRate > 0 {
					sampleRate = msg.SampleRate
				}
				log.Printf("Started: targetLang=%s, sourceLang=%s, sampleRate=%d", targetLang, sourceLang, sampleRate)
				sendJSON(wsEvent{Type: "info", Text: "started"})
			case "stop":
				// Finalize any pending partial before stopping
				mu.Lock()
				if lastPartial != "" {
					finalText := lastPartial
					id := nextID
					nextID++
					lastPartial = ""
					stableSince = time.Time{}
					mu.Unlock()

					sendJSON(wsEvent{Type: "final", ID: id, Text: finalText})
					tr, _ := s.tr.Translate(finalText, targetLang)
					sendJSON(wsEvent{Type: "translation", ID: id, Text: tr})
				} else {
					mu.Unlock()
				}
				started = false
				sendJSON(wsEvent{Type: "info", Text: "stopped"})
			}
			continue
		}

		if mt == websocket.BinaryMessage {
			// data is Int16Array buffer from browser
			if len(data)%2 != 0 {
				log.Printf("Binary data size not even: %d bytes", len(data))
				continue
			}
			samples := make([]int16, len(data)/2)
			_ = binary.Read(bytes.NewReader(data), binary.LittleEndian, &samples)
			log.Printf("Received %d samples (%d bytes) from browser", len(samples), len(data))
			ring.Write(samples)
		}
	}
}
