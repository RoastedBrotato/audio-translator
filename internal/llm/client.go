package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Client is an HTTP client for the LLM service
type Client struct {
	BaseURL string
	HTTP    *http.Client
}

// New creates a new LLM service client with a longer timeout for generation
func New(baseURL string) *Client {
	return &Client{
		BaseURL: baseURL,
		HTTP: &http.Client{
			Timeout: 120 * time.Second, // 2 minutes for LLM generation
		},
	}
}

// GenerateRequest represents a request to generate text from the LLM
type GenerateRequest struct {
	Prompt      string  `json:"prompt"`
	Context     string  `json:"context"`
	MaxTokens   int     `json:"max_tokens,omitempty"`
	Temperature float64 `json:"temperature,omitempty"`
	Language    string  `json:"language,omitempty"`
}

// GenerateResponse represents the response from the LLM
type GenerateResponse struct {
	Response string `json:"response"`
	Model    string `json:"model"`
}

// Generate generates a response from the LLM based on the prompt and context (default English)
func (c *Client) Generate(prompt, context string, maxTokens int, temperature float64) (string, error) {
	return c.GenerateWithLanguage(prompt, context, "en", maxTokens, temperature)
}

// GenerateWithLanguage generates a response from the LLM in the specified language
func (c *Client) GenerateWithLanguage(prompt, context, language string, maxTokens int, temperature float64) (string, error) {
	reqBody := GenerateRequest{
		Prompt:      prompt,
		Context:     context,
		MaxTokens:   maxTokens,
		Temperature: temperature,
		Language:    language,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	resp, err := c.HTTP.Post(
		c.BaseURL+"/generate",
		"application/json",
		bytes.NewReader(jsonData),
	)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("llm service returned status %d", resp.StatusCode)
	}

	var result GenerateResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	return result.Response, nil
}
