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
	Model          string
	BaseURL        string
	ModelsURL      string // Added for testing
	FallbackModels []string
	currentIdx     int
}

func NewOpenRouter(apiKey, model string) *OpenRouter {
	return &OpenRouter{
		APIKey:    apiKey,
		Model:     model,
		BaseURL:   defaultOpenRouterURL,
		ModelsURL: modelsURL,
	}
}

// FetchFreeModels programmatically finds all 0-cost models.
func (o *OpenRouter) FetchFreeModels(ctx context.Context) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", o.ModelsURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var res struct {
		Data []struct {
			ID      string `json:"id"`
			Pricing struct {
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
	// Exclude very small models that might struggle with ReAct reasoning
	blacklist := []string{"1b", "0.5b", "phi-3-mini", "tiny", "tinyllama"}

	for _, m := range res.Data {
		// Strictly filter for models that have 0 pricing AND the :free suffix
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

	// Move the initially preferred model to the front if it's free
	for i, f := range free {
		if f == o.Model {
			free[0], free[i] = free[i], free[0]
			break
		}
	}

	o.FallbackModels = free
	return free, nil
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
	start := time.Now()
	var lastErr error

	// 1. Prepare models to rotate through
	modelsToTry := o.FallbackModels
	if len(modelsToTry) == 0 {
		modelsToTry = []string{o.Model}
	}

	// 2. Rotate through models
	for i := o.currentIdx; i < len(modelsToTry); i++ {
		currentModel := modelsToTry[i]
		o.Model = currentModel
		o.currentIdx = i

		delay := 4 * time.Second
		// Inner loop for exponential backoff on the current model
		for retry := 0; retry < 3; retry++ {
			res, usage, err := o.doChatWithFallback(ctx, messages)
			if err == nil {
				fmt.Printf("\r[LLM Response] Received after %v.\n", time.Since(start).Round(time.Millisecond))
				return res, usage, nil
			}

			lastErr = err
			errStr := strings.ToLower(err.Error())

			// Check for 429 (Too Many Requests) or 403 (Forbidden/Provider Error)
			if strings.Contains(errStr, "429") || strings.Contains(errStr, "403") {
				reason := "Rate Limited"
				if strings.Contains(errStr, "403") {
					reason = "Provider Error"
				}
				fmt.Printf("\n[%s (%s) on %s] Retrying in %v... (Attempt %d/3)\n", reason, errStr, currentModel, delay, retry+1)
				select {
				case <-ctx.Done():
					return "", Usage{}, ctx.Err()
				case <-time.After(delay):
					delay *= 2 // Exponential backoff
					continue
				}
			}
			
			// Non-retriable error
			break
		}

		// If it's a 429/403 after 3 retries, rotate to the next model in the list
		errStr := strings.ToLower(lastErr.Error())
		if (strings.Contains(errStr, "429") || strings.Contains(errStr, "403")) && i < len(modelsToTry)-1 {
			fmt.Printf("\n[Model Switch] Rotating from %s due to persistent provider error/limit.\n", currentModel)
			continue
		}

		// If we reached here, it's either success (returned above), a non-429 error, or we exhausted all models.
		return "", Usage{}, lastErr
	}

	return "", Usage{}, fmt.Errorf("exceeded all retry attempts and all fallback models: %w", lastErr)
}

func (o *OpenRouter) doChatWithFallback(ctx context.Context, messages []Message) (string, Usage, error) {
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
