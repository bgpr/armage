package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
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
	Usage Usage `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (o *OpenRouter) Chat(ctx context.Context, messages []Message) (string, Usage, error) {
	// 1. Try with original messages (respecting 'system' role)
	res, usage, err := o.doRequest(ctx, messages)
	if err == nil {
		return res, usage, nil
	}

	// 2. If it's a 400 error mentioning 'system' role or 'developer instruction', retry with fallback
	errStr := strings.ToLower(err.Error())
	if strings.Contains(errStr, "400") && (strings.Contains(errStr, "system") || strings.Contains(errStr, "developer instruction")) {
		fallbackMessages := make([]Message, len(messages))
		for i, m := range messages {
			if m.Role == "system" {
				fallbackMessages[i] = Message{
					Role:    "user",
					Content: "System Instructions: " + m.Content,
				}
			} else {
				fallbackMessages[i] = m
			}
		}
		// Retry with user-role instructions
		return o.doRequest(ctx, fallbackMessages)
	}

	return "", Usage{}, err
}

func (o *OpenRouter) doRequest(ctx context.Context, messages []Message) (string, Usage, error) {
	reqBody := openRouterRequest{
		Model:    o.Model,
		Messages: messages,
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", Usage{}, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", o.BaseURL, bytes.NewBuffer(body))
	if err != nil {
		return "", Usage{}, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+o.APIKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", Usage{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errBody bytes.Buffer
		errBody.ReadFrom(resp.Body)
		// Return the error body as part of the error message for inspection
		return "", Usage{}, fmt.Errorf("HTTP %d: %s", resp.StatusCode, errBody.String())
	}

	var orResp openRouterResponse
	if err := json.NewDecoder(resp.Body).Decode(&orResp); err != nil {
		return "", Usage{}, err
	}

	if orResp.Error != nil {
		return "", Usage{}, fmt.Errorf("OpenRouter error: %s", orResp.Error.Message)
	}

	if len(orResp.Choices) == 0 {
		return "", Usage{}, fmt.Errorf("no choices returned")
	}

	return orResp.Choices[0].Message.Content, orResp.Usage, nil
}
