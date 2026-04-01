package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Scrubber interface for redacting PII/keys.
type Scrubber interface {
	Scrub(ctx context.Context, text string) (string, error)
}

// LocalScrubber uses a local OpenAI-compatible server (llama.cpp) to redact text.
type LocalScrubber struct {
	BaseURL    string
	Model      string // Optional model name
	BinaryPath string
	ModelPath  string
	started    bool
	muStart    sync.Mutex
	cmd        *exec.Cmd // Track the running command
}

func (l *LocalScrubber) EnsureRunning(ctx context.Context) error {
	l.muStart.Lock()
	defer l.muStart.Unlock()

	// 1. Simple health check
	client := &http.Client{Timeout: 2 * time.Second}
	// llama-server has a dedicated /health endpoint. 
	// We derive it from the BaseURL by replacing the path.
	healthURL := strings.Replace(l.BaseURL, "/v1/chat/completions", "/health", 1)
	if !strings.Contains(healthURL, "/health") {
		// Fallback if URL doesn't match expected pattern
		healthURL = l.BaseURL 
	}

	resp, err := client.Get(healthURL)
	if err == nil && resp.StatusCode == http.StatusOK {
		resp.Body.Close()
		return nil
	}

	if l.BinaryPath == "" || l.ModelPath == "" {
		return fmt.Errorf("local scrubber not reachable and binary/model paths are not configured")
	}

	// 2. Start the server
	fmt.Printf("Starting local scrubber server: %s --model %s\n", l.BinaryPath, l.ModelPath)
	
	// Create a log file for the server
	logFile, _ := os.OpenFile("llama_server.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	
	// REDUCED THREADS: Using 4 threads instead of the default 8+ to leave room for the shell.
	l.cmd = exec.Command(l.BinaryPath, "--model", l.ModelPath, "--port", "8080", "--n-gpu-layers", "0", "--threads", "4")
	if logFile != nil {
		l.cmd.Stdout = logFile
		l.cmd.Stderr = logFile
	}
	
	err = l.cmd.Start()
	if err != nil {
		return fmt.Errorf("failed to start llama-server: %w", err)
	}

	// 3. Wait for the server to be ready
	fmt.Print("Waiting for local scrubber to initialize...")
	for i := 0; i < 60; i++ { // Increased to 60 seconds for slow mobile initialization
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(1 * time.Second):
			resp, err := client.Get(healthURL)
			if err == nil && resp.StatusCode == http.StatusOK {
				resp.Body.Close()
				fmt.Println(" Ready.")
				return nil
			}
		}
	}

	return fmt.Errorf("timeout waiting for local scrubber to start after 60s")
}

// Stop terminates the auto-started llama-server.
func (l *LocalScrubber) Stop() error {
	l.muStart.Lock()
	defer l.muStart.Unlock()

	if l.cmd == nil || l.cmd.Process == nil {
		return nil
	}

	fmt.Println("Stopping local scrubber server...")
	err := l.cmd.Process.Kill()
	l.cmd.Wait() // Wait for cleanup
	l.cmd = nil
	return err
}

func (l *LocalScrubber) Scrub(ctx context.Context, text string) (string, error) {
	if text == "" {
		return "", nil
	}

	if err := l.EnsureRunning(ctx); err != nil {
		return "", fmt.Errorf("scrubber initialization failed: %w", err)
	}

	// ULTRA-SIMPLE PROMPT: No instruction tags, just a direct order.
	prompt := fmt.Sprintf(`Rewrite the following text. Replace any names, emails, or keys with REDACTED_NAME, REDACTED_EMAIL, or REDACTED_KEY. 
Output the rewritten text between ---SAFE--- and ---END--- markers. 
Do not explain.

TEXT:
%s`, text)

	reqBody := struct {
		Model    string    `json:"model"`
		Messages []Message `json:"messages"`
	}{
		Model: l.Model,
		Messages: []Message{
			{Role: "user", Content: prompt},
		},
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return text, nil
	}

	req, err := http.NewRequestWithContext(ctx, "POST", l.BaseURL, bytes.NewBuffer(body))
	if err != nil {
		return text, nil
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return text, nil
	}
	defer resp.Body.Close()

	var res struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return text, nil
	}

	if len(res.Choices) == 0 {
		return text, nil
	}

	content := res.Choices[0].Message.Content
	
	// LOG FOR DEBUGGING
	debugLog, _ := os.OpenFile("scrubber_debug.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if debugLog != nil {
		fmt.Fprintf(debugLog, "\n--- ORIGINAL ---\n%s\n--- RAW OUTPUT ---\n%s\n", text, content)
		debugLog.Close()
	}

	// ROBUST EXTRACTION: Look for common markers or XML-ish tags
	re := regexp.MustCompile(`(?s)(?:---SAFE---|(?:\<|\[)safe_text(?:\>|\]))(.*?)(?:---END---|(?:\<|\[)/safe_text(?:\>|\])|$)`)
	match := re.FindStringSubmatch(content)
	if len(match) > 1 {
		return strings.TrimSpace(match[1]), nil
	}

	// PROTECTION: Aggressively strip prompt tags if they leaked
	clean := content
	junk := []string{"[INST]", "[/INST]", "<<SYS>>", "<</SYS>>", "safe_text", "SAFE", "END", "TEXT:", "---", "[", "]", "<", ">"}
	for _, s := range junk {
		clean = strings.ReplaceAll(clean, s, "")
	}
	clean = strings.TrimSpace(clean)

	// Final Fallback: If it's empty or looks like a refusal, use original
	lowered := strings.ToLower(clean)
	if len(clean) < 2 || strings.Contains(lowered, "i cannot") || strings.Contains(lowered, "i refuse") || strings.Contains(lowered, "i'm sorry") {
		return text, nil
	}

	return clean, nil
}

// ScrubbingLLM is a decorator that redacts messages before sending them to the inner LLM.
type ScrubbingLLM struct {
	Inner     LLM
	Scrubber  Scrubber
	cache     map[string]string // Memoization: map[original]scrubbed
	cachePath string
	mu        sync.RWMutex
}

func NewScrubbingLLM(inner LLM, scrubber Scrubber, cachePath string) *ScrubbingLLM {
	s := &ScrubbingLLM{
		Inner:     inner,
		Scrubber:  scrubber,
		cache:     make(map[string]string),
		cachePath: cachePath,
	}
	s.loadCache()
	return s
}

func (s *ScrubbingLLM) loadCache() {
	if s.cachePath == "" {
		return
	}
	data, err := os.ReadFile(s.cachePath)
	if err != nil {
		return
	}
	s.mu.Lock()
	json.Unmarshal(data, &s.cache)
	s.mu.Unlock()
}

func (s *ScrubbingLLM) saveCache() {
	if s.cachePath == "" {
		return
	}
	s.mu.RLock()
	data, err := json.Marshal(s.cache)
	s.mu.RUnlock()
	if err == nil {
		os.WriteFile(s.cachePath, data, 0644)
	}
}

func (s *ScrubbingLLM) Model() string {
	return s.Inner.Model()
}

func (s *ScrubbingLLM) Chat(ctx context.Context, messages []Message) (string, Usage, error) {
	scrubbedMessages := make([]Message, len(messages))
	
	type work struct {
		index   int
		content string
	}
	var pending []work

	// Regex to identify tool calls and code blocks that should NEVER be scrubbed/mangled
	techRegex := regexp.MustCompile(`(?s)(<tool_call>.*?</tool_call>|Action:\s+\w+\(.*?\)|` + "`" + `{3}.*?` + "`" + `{3})`)

	for i, m := range messages {
		// SKIP SCRUBBING FOR:
		// 1. Trusted system prompts
		// 2. Assistant messages
		// 3. Technical observations (source code, dir listings, etc.)
		// 4. Technical nudges or very short messages
		isSystem := m.Role == "system" && strings.HasPrefix(m.Content, "You are Armage")
		isObservation := strings.HasPrefix(m.Content, "Observation") || strings.HasPrefix(m.Content, "Observations")
		isShort := len(m.Content) < 20
		isNudge := strings.Contains(m.Content, "Please continue")
		
		if isSystem || m.Role == "assistant" || isObservation || (m.Role == "user" && (isShort || isNudge)) {
			scrubbedMessages[i] = m
			continue
		}

		s.mu.RLock()
		cached, ok := s.cache[m.Content]
		s.mu.RUnlock()

		if ok {
			scrubbedMessages[i] = Message{Role: m.Role, Content: cached}
		} else {
			// TAG-AWARE SCRUBBING:
			// We only scrub the text OUTSIDE of technical tags.
			original := m.Content
			parts := techRegex.Split(original, -1)
			matches := techRegex.FindAllString(original, -1)

			// If it's a very complex message, just scrub the whole thing but carefully
			// For now, if it has tech tags, we'll be surgical
			if len(matches) > 0 {
				var finalContent strings.Builder
				for idx, part := range parts {
					if len(strings.TrimSpace(part)) > 10 {
						scrubbedPart, err := s.Scrubber.Scrub(ctx, part)
						if err == nil {
							finalContent.WriteString(scrubbedPart)
						} else {
							finalContent.WriteString(part)
						}
					} else {
						finalContent.WriteString(part)
					}
					if idx < len(matches) {
						finalContent.WriteString(matches[idx]) // Keep tech tags AS-IS
					}
				}
				scrubbed := finalContent.String()
				s.mu.Lock()
				s.cache[original] = scrubbed
				s.mu.Unlock()
				scrubbedMessages[i] = Message{Role: m.Role, Content: scrubbed}
			} else {
				pending = append(pending, work{i, m.Content})
			}
		}
	}

	// Process simple pending messages in parallel
	if len(pending) > 0 {
		start := time.Now()
		fmt.Printf("\r[Privacy Shield] Scrubbing %d new messages... ", len(pending))
		
		var wg sync.WaitGroup
		errChan := make(chan error, len(pending))

		for _, w := range pending {
			wg.Add(1)
			go func(work work) {
				defer wg.Done()
				scrubbed, err := s.Scrubber.Scrub(ctx, work.content)
				if err != nil {
					errChan <- err
					return
				}
				
				s.mu.Lock()
				s.cache[work.content] = scrubbed
				s.mu.Unlock()
				
				scrubbedMessages[work.index] = Message{
					Role:    messages[work.index].Role,
					Content: scrubbed,
				}
			}(w)
		}

		wg.Wait()
		close(errChan)

		if err := <-errChan; err != nil {
			return "", Usage{}, fmt.Errorf("parallel scrubbing failed: %w", err)
		}

		fmt.Printf("Done (%v).\n", time.Since(start).Round(time.Millisecond))
		s.saveCache()
	}

	return s.Inner.Chat(ctx, scrubbedMessages)
}
