package asr

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type Client struct {
	BaseURL string
	HTTP    *http.Client
}

func New(baseURL string) *Client {
	return &Client{
		BaseURL: baseURL,
		HTTP:    &http.Client{Timeout: 120 * time.Second},
	}
}

type Resp struct {
	Text string `json:"text"`
}

// Minimal WAV (PCM16 mono) wrapper
func pcm16ToWav(pcm []int16, sampleRate int) ([]byte, error) {
	dataBytes := len(pcm) * 2
	var b bytes.Buffer

	// RIFF header
	b.WriteString("RIFF")
	_ = binary.Write(&b, binary.LittleEndian, uint32(36+dataBytes))
	b.WriteString("WAVE")

	// fmt chunk
	b.WriteString("fmt ")
	_ = binary.Write(&b, binary.LittleEndian, uint32(16))           // PCM
	_ = binary.Write(&b, binary.LittleEndian, uint16(1))            // audio format = PCM
	_ = binary.Write(&b, binary.LittleEndian, uint16(1))            // channels
	_ = binary.Write(&b, binary.LittleEndian, uint32(sampleRate))   // sample rate
	_ = binary.Write(&b, binary.LittleEndian, uint32(sampleRate*2)) // byte rate
	_ = binary.Write(&b, binary.LittleEndian, uint16(2))            // block align
	_ = binary.Write(&b, binary.LittleEndian, uint16(16))           // bits per sample

	// data chunk
	b.WriteString("data")
	_ = binary.Write(&b, binary.LittleEndian, uint32(dataBytes))

	// PCM data
	for _, s := range pcm {
		_ = binary.Write(&b, binary.LittleEndian, s)
	}
	return b.Bytes(), nil
}

func (c *Client) TranscribePCM16(pcm []int16, sampleRate int) (string, error) {
	return c.TranscribePCM16WithLang(pcm, sampleRate, "")
}

func (c *Client) TranscribePCM16WithLang(pcm []int16, sampleRate int, language string) (string, error) {
	wav, err := pcm16ToWav(pcm, sampleRate)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", c.BaseURL+"/transcribe", bytes.NewReader(wav))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "audio/wav")
	if language != "" {
		req.Header.Set("x-language", language)
	}

	res, err := c.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	if res.StatusCode >= 300 {
		return "", fmt.Errorf("asr status: %s", res.Status)
	}

	var r Resp
	if err := json.NewDecoder(res.Body).Decode(&r); err != nil {
		return "", err
	}
	return r.Text, nil
}

// TranscribeWAV transcribes a complete WAV file (for batch processing)
func (c *Client) TranscribeWAV(wavData []byte, language string) (string, error) {
	req, err := http.NewRequest("POST", c.BaseURL+"/transcribe", bytes.NewReader(wavData))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "audio/wav")
	if language != "" {
		req.Header.Set("x-language", language)
	}

	res, err := c.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	if res.StatusCode >= 300 {
		return "", fmt.Errorf("asr status: %s", res.Status)
	}

	var r Resp
	if err := json.NewDecoder(res.Body).Decode(&r); err != nil {
		return "", err
	}
	return r.Text, nil
}

// DetectLanguageResponse represents the response from language detection
type DetectLanguageResponse struct {
	Language string `json:"language"`
	Text     string `json:"text"`
	Segments []struct {
		Start    float64 `json:"start"`
		End      float64 `json:"end"`
		Text     string  `json:"text"`
		Language string  `json:"language,omitempty"`
	} `json:"segments,omitempty"`
}

// DetectLanguage detects the language of the audio without requiring a language hint
func (c *Client) DetectLanguage(wavData []byte) (string, error) {
	req, err := http.NewRequest("POST", c.BaseURL+"/detect-language", bytes.NewReader(wavData))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "audio/wav")

	res, err := c.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	if res.StatusCode >= 300 {
		return "", fmt.Errorf("language detection status: %s", res.Status)
	}

	var r DetectLanguageResponse
	if err := json.NewDecoder(res.Body).Decode(&r); err != nil {
		return "", err
	}
	return r.Language, nil
}
