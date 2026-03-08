package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

const defaultOpenRouterURL = "https://openrouter.ai/api/v1/chat/completions"

type OpenRouter struct {
	APIKey  string
	Model   string
	BaseURL string
}

func NewOpenRouter(apiKey, model string) *OpenRouter {
	return &OpenRouter{
		APIKey:  apiKey,
		Model:   model,
		BaseURL: defaultOpenRouterURL,
	}
}

type openRouterRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
}

type openRouterResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (o *OpenRouter) Chat(ctx context.Context, messages []Message) (string, error) {
	reqBody := openRouterRequest{
		Model:    o.Model,
		Messages: messages,
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", o.BaseURL, bytes.NewBuffer(body))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+o.APIKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errBody bytes.Buffer
		errBody.ReadFrom(resp.Body)
		return "", fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, errBody.String())
	}

	var orResp openRouterResponse
	if err := json.NewDecoder(resp.Body).Decode(&orResp); err != nil {
		return "", err
	}

	if orResp.Error != nil {
		return "", fmt.Errorf("OpenRouter error: %s", orResp.Error.Message)
	}

	if len(orResp.Choices) == 0 {
		return "", fmt.Errorf("no choices returned")
	}

	return orResp.Choices[0].Message.Content, nil
}
