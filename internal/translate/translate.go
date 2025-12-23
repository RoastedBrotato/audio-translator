package translate

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type Translator interface {
	Translate(text, targetLang string) (string, error)
	TranslateWithSource(text, sourceLang, targetLang string) (string, error)
}

type Stub struct{}

func (s Stub) Translate(text, targetLang string) (string, error) {
	// MVP: just echo. Replace with real translator later.
	return "[" + targetLang + "] " + text, nil
}

func (s Stub) TranslateWithSource(text, sourceLang, targetLang string) (string, error) {
	return "[" + sourceLang + " -> " + targetLang + "] " + text, nil
}

// HTTPTranslator calls a translation service over HTTP
type HTTPTranslator struct {
	BaseURL    string
	HTTPClient *http.Client
}

type translateRequest struct {
	Text       string `json:"text"`
	SourceLang string `json:"source_lang"`
	TargetLang string `json:"target_lang"`
}

type translateResponse struct {
	Translation string `json:"translation"`
}

func (h *HTTPTranslator) Translate(text, targetLang string) (string, error) {
	// Default to auto-detect source language
	return h.TranslateWithSource(text, "auto", targetLang)
}

func (h *HTTPTranslator) TranslateWithSource(text, sourceLang, targetLang string) (string, error) {
	if text == "" {
		return "", nil
	}

	req := translateRequest{
		Text:       text,
		SourceLang: sourceLang,
		TargetLang: targetLang,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequest("POST", h.BaseURL+"/translate", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	client := h.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("translation service returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result translateResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}

	return result.Translation, nil
}
