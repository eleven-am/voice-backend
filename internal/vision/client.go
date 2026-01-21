package vision

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type Client struct {
	httpClient *http.Client
	baseURL    string
	model      string
}

func NewClient(cfg Config) *Client {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	return &Client{
		httpClient: &http.Client{Timeout: timeout},
		baseURL:    cfg.OllamaURL,
		model:      cfg.Model,
	}
}

type ollamaRequest struct {
	Model  string   `json:"model"`
	Prompt string   `json:"prompt"`
	Images []string `json:"images,omitempty"`
	Stream bool     `json:"stream"`
}

type ollamaResponse struct {
	Response string `json:"response"`
	Done     bool   `json:"done"`
}

func (c *Client) Analyze(ctx context.Context, req AnalyzeRequest) (*AnalyzeResponse, error) {
	fmt.Printf("VISION DEBUG client.Analyze: called with frame=%v\n", req.Frame != nil)
	if req.Frame == nil || len(req.Frame.Data) == 0 {
		fmt.Printf("VISION DEBUG client.Analyze: no frame data provided\n")
		return nil, fmt.Errorf("no frame data provided")
	}

	fmt.Printf("VISION DEBUG client.Analyze: frame data size=%d bytes\n", len(req.Frame.Data))

	prompt := req.Prompt
	if prompt == "" {
		prompt = "Describe what you see in this image concisely. Focus on the main content and any text visible."
	}

	imageB64 := base64.StdEncoding.EncodeToString(req.Frame.Data)

	ollamaReq := ollamaRequest{
		Model:  c.model,
		Prompt: prompt,
		Images: []string{imageB64},
		Stream: false,
	}

	body, err := json.Marshal(ollamaReq)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	fmt.Printf("VISION DEBUG client.Analyze: sending to Ollama at %s, model=%s\n", c.baseURL, c.model)

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		fmt.Printf("VISION DEBUG client.Analyze: Ollama request error: %v\n", err)
		return nil, fmt.Errorf("ollama request: %w", err)
	}
	defer resp.Body.Close()

	fmt.Printf("VISION DEBUG client.Analyze: Ollama responded with status=%d\n", resp.StatusCode)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama returned status %d", resp.StatusCode)
	}

	var ollamaResp ollamaResponse
	if err := json.NewDecoder(resp.Body).Decode(&ollamaResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	fmt.Printf("VISION DEBUG client.Analyze: Ollama response length=%d chars\n", len(ollamaResp.Response))
	fmt.Printf("VISION DEBUG client.Analyze: Ollama response: %s\n", ollamaResp.Response)

	return &AnalyzeResponse{
		Description: ollamaResp.Response,
		Timestamp:   req.Frame.Timestamp,
	}, nil
}

func (c *Client) IsAvailable(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/api/tags", nil)
	if err != nil {
		return false
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}
