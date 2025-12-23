package tts

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"time"
)

// Client handles text-to-speech requests
type Client struct {
	BaseURL string
	HTTP    *http.Client
}

// New creates a new TTS client
func New(baseURL string) *Client {
	return &Client{
		BaseURL: baseURL,
		HTTP:    &http.Client{Timeout: 300 * time.Second}, // 5 minutes for XTTS v2
	}
}

// SynthesizeRequest represents a TTS request
type SynthesizeRequest struct {
	Text     string `json:"text"`
	Language string `json:"language"`
}

// Synthesize converts text to speech audio (MP3)
func (c *Client) Synthesize(text, language string) ([]byte, error) {
	if text == "" {
		return nil, fmt.Errorf("text cannot be empty")
	}

	reqBody := SynthesizeRequest{
		Text:     text,
		Language: language,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", c.BaseURL+"/synthesize", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("TTS service returned %d: %s", resp.StatusCode, string(respBody))
	}

	// Read MP3 audio data
	audioData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read audio data: %w", err)
	}

	return audioData, nil
}

// SynthesizeWithVoice converts text to speech with voice cloning from reference audio
func (c *Client) SynthesizeWithVoice(text, language string, referenceAudio []byte) ([]byte, error) {
	if text == "" {
		return nil, fmt.Errorf("text cannot be empty")
	}
	if len(referenceAudio) == 0 {
		return nil, fmt.Errorf("reference audio cannot be empty")
	}

	// Create multipart form data
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Add text field
	if err := writer.WriteField("text", text); err != nil {
		return nil, fmt.Errorf("write text field: %w", err)
	}

	// Add language field
	if err := writer.WriteField("language", language); err != nil {
		return nil, fmt.Errorf("write language field: %w", err)
	}

	// Add reference audio file
	part, err := writer.CreateFormFile("reference_audio", "reference.wav")
	if err != nil {
		return nil, fmt.Errorf("create form file: %w", err)
	}
	if _, err := part.Write(referenceAudio); err != nil {
		return nil, fmt.Errorf("write audio data: %w", err)
	}

	contentType := writer.FormDataContentType()
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("close writer: %w", err)
	}

	req, err := http.NewRequest("POST", c.BaseURL+"/synthesize_with_voice", body)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", contentType)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("TTS service returned %d: %s", resp.StatusCode, string(respBody))
	}

	// Read WAV audio data
	audioData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read audio data: %w", err)
	}

	return audioData, nil
}
