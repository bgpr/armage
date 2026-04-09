package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const defaultOpenRouterURL = "https://openrouter.ai/api/v1/chat/completions"
const modelsURL = "https://openrouter.ai/api/v1/models"

type OpenRouter struct {
	APIKey         string
	CurrentModel   string 
	BaseURL        string
	ModelsURL      string 
	FallbackModels []string
	ModelMetadata  map[string]int // Map of model ID to context_length
	currentIdx     int
	Logger         Logger // Added for TUI-safe logging
	HTTPClient     *http.Client
}

func NewOpenRouter(apiKey, model string) *OpenRouter {
	return &OpenRouter{
		APIKey:       apiKey,
		CurrentModel: model,
		BaseURL:      defaultOpenRouterURL,
		ModelsURL:    modelsURL,
		ModelMetadata: make(map[string]int),
		HTTPClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// FetchFreeModels programmatically finds all 0-cost models and their context limits.
func (o *OpenRouter) FetchFreeModels(ctx context.Context) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", o.ModelsURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := o.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var res struct {
		Data []struct {
			ID            string `json:"id"`
			ContextLength int    `json:"context_length"`
			Pricing       struct {
				Prompt     string `json:"prompt"`
				Completion string `json:"completion"`
				Request    string `json:"request"`
			} `json:"pricing"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}

	var free []string
	blacklist := []string{"1b", "0.5b", "phi-3-mini", "tiny", "tinyllama"}

	for _, m := range res.Data {
		// Store context length for ALL models we see
		o.ModelMetadata[m.ID] = m.ContextLength

		if m.Pricing.Prompt == "0" && m.Pricing.Completion == "0" && strings.HasSuffix(m.ID, ":free") {
			isBlacklisted := false
			lowered := strings.ToLower(m.ID)
			for _, b := range blacklist {
				if strings.Contains(lowered, b) {
					isBlacklisted = true
					break
				}
			}
			if !isBlacklisted {
				free = append(free, m.ID)
			}
		}
	}

	for i, f := range free {
		if f == o.CurrentModel {
			free[0], free[i] = free[i], free[0]
			break
		}
	}

	o.FallbackModels = free
	return free, nil
}

func (o *OpenRouter) ContextWindow() int {
	if limit, ok := o.ModelMetadata[o.CurrentModel]; ok && limit > 0 {
		return limit
	}
	return 4096 // Absolute fallback
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

func (o *OpenRouter) Model() string {
	return o.CurrentModel
}

func (o *OpenRouter) Chat(ctx context.Context, messages []Message) (string, Usage, error) {
	var lastErr error

	modelsToTry := o.FallbackModels
	if len(modelsToTry) == 0 {
		modelsToTry = []string{o.CurrentModel}
	}

	for i := o.currentIdx; i < len(modelsToTry); i++ {
		currentModel := modelsToTry[i]
		o.CurrentModel = currentModel
		o.currentIdx = i

		delay := 4 * time.Second
		for retry := 0; retry < 3; retry++ {
			res, usage, err := o.doChatWithFallback(ctx, messages)
			if err == nil {
				return res, usage, nil
			}

			lastErr = err
			errStr := strings.ToLower(err.Error())

			if strings.Contains(errStr, "429") || strings.Contains(errStr, "403") || strings.Contains(errStr, "timeout") {
				reason := "Rate Limited"
				if strings.Contains(errStr, "403") {
					reason = "Provider Error"
				} else if strings.Contains(errStr, "timeout") {
					reason = "Timeout"
				}
				msg := fmt.Sprintf("[%s on %s] Retrying in %v... (Attempt %d/3)", reason, currentModel, delay, retry+1)
				if o.Logger != nil {
					o.Logger(msg)
				}
				
				select {
				case <-ctx.Done():
					return "", Usage{}, ctx.Err()
				case <-time.After(delay):
					delay *= 2 
					continue
				}
			}
			break
		}

		errStr := strings.ToLower(lastErr.Error())
		if (strings.Contains(errStr, "429") || strings.Contains(errStr, "403") || strings.Contains(errStr, "timeout")) && i < len(modelsToTry)-1 {
			msg := fmt.Sprintf("[Model Switch] Rotating from %s due to persistent error.", currentModel)
			if o.Logger != nil {
				o.Logger(msg)
			}
			continue
		}

		return "", Usage{}, lastErr
	}

	return "", Usage{}, fmt.Errorf("exceeded all retry attempts and all fallback models: %w", lastErr)
}

func (o *OpenRouter) doChatWithFallback(ctx context.Context, messages []Message) (string, Usage, error) {
	res, usage, err := o.doRequest(ctx, messages)
	if err == nil {
		return res, usage, nil
	}

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
		return o.doRequest(ctx, fallbackMessages)
	}

	return "", Usage{}, err
}

func (o *OpenRouter) doRequest(ctx context.Context, messages []Message) (string, Usage, error) {
	reqBody := openRouterRequest{
		Model:    o.CurrentModel,
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

	resp, err := o.HTTPClient.Do(req)
	if err != nil {
		return "", Usage{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errBody bytes.Buffer
		errBody.ReadFrom(resp.Body)
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
