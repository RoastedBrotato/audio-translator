package embedding

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Client is an HTTP client for the embedding service
type Client struct {
	BaseURL string
	HTTP    *http.Client
}

// New creates a new embedding service client
func New(baseURL string) *Client {
	return &Client{
		BaseURL: baseURL,
		HTTP: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// EmbedRequest represents a request to embed a single text
type EmbedRequest struct {
	Text string `json:"text"`
}

// EmbedResponse represents the response from embedding a single text
type EmbedResponse struct {
	Embedding []float32 `json:"embedding"`
	Dimension int       `json:"dimension"`
}

// EmbedBatchRequest represents a request to embed multiple texts
type EmbedBatchRequest struct {
	Texts []string `json:"texts"`
}

// EmbedBatchResponse represents the response from embedding multiple texts
type EmbedBatchResponse struct {
	Embeddings [][]float32 `json:"embeddings"`
	Dimension  int         `json:"dimension"`
	Count      int         `json:"count"`
}

// Embed generates an embedding for a single text
func (c *Client) Embed(text string) ([]float32, error) {
	reqBody := EmbedRequest{Text: text}
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	resp, err := c.HTTP.Post(
		c.BaseURL+"/embed",
		"application/json",
		bytes.NewReader(jsonData),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("embedding service returned status %d", resp.StatusCode)
	}

	var result EmbedResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return result.Embedding, nil
}

// EmbedBatch generates embeddings for multiple texts (more efficient than calling Embed multiple times)
func (c *Client) EmbedBatch(texts []string) ([][]float32, error) {
	reqBody := EmbedBatchRequest{Texts: texts}
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	resp, err := c.HTTP.Post(
		c.BaseURL+"/embed-batch",
		"application/json",
		bytes.NewReader(jsonData),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("embedding service returned status %d", resp.StatusCode)
	}

	var result EmbedBatchResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return result.Embeddings, nil
}
