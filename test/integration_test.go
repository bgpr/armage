package test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/user/armage/pkg/agent"
	"github.com/user/armage/pkg/provider"
)

func TestIntegrationFullCycle(t *testing.T) {
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		t.Skip("Skipping integration test: OPENROUTER_API_KEY not set")
	}

	// 1. Setup with a FREE model
	model := os.Getenv("OPENROUTER_MODEL")
	if model == "" {
		model = "meta-llama/llama-3.2-3b-instruct:free"
	}
	llm := provider.NewOpenRouter(apiKey, model)
	reg := agent.NewRegistry()
	reg.Register(&agent.ShellTool{})
	reg.Register(&agent.ReadTool{})
	reg.Register(&agent.WriteTool{})
	reg.Register(&agent.SearchTool{})
	reg.Register(&agent.DiffTool{})
	reg.Register(&agent.ListDirTool{})
	reg.Register(&agent.SymbolsTool{})
	reg.Register(&agent.ApplyPatchTool{})

	a := agent.New(llm, reg)
	reg.Register(&agent.PinTool{Agent: a})
	reg.Register(&agent.PlanningTool{Agent: a})

	a.RequireApproval = true
	a.AddSystemPrompt(`You are Armage, an expert coding agent for Termux. 
Follow the ReAct pattern strictly: 
Thought: [Your reasoning]
Action: ToolName([JSON Arguments])

Available Tools:
- shell: Executes a shell command and returns the output.
- list_dir: {"path": "...", "depth": 1}. Lists files/directories.
- get_symbols: {"path": "..."}. Lists functions, classes, and types in a file.
- propose_plan: {"plan": "..."}. Documents a strategy in PLAN.md and pins it.
- pin_file: {"path": "..."}. Pins a file to your history permanently.
- read_file: {"path": "...", "start": 1, "end": 10}. Reads a file with line numbers.
- write_file: {"path": "...", "content": "..."}. Writes content to a file atomically.
- grep_search: {"pattern": "...", "path": "..."}. Searches for a pattern in files.
- edit_file_diff: {"path": "...", "find": "...", "replace": "..."}. Surgically updates a file.
- apply_patch: {"path": "...", "patch": "..."}. Applies a standard unified diff (patch).

Example:
Thought: I need a strategy first.
Action: propose_plan({"plan": "1. Research\n2. Implement"})
`)

	fmt.Printf("\n--- Multi-Step Search & Edit Integration Test ---\n")

	// 2. Task: Pin, Search, Read, then Edit a specific file.
	ctx := context.Background()
	task := "Propose a plan in PLAN.md for this task. Pin TODO.md to your context. Explore the project structure with list_dir. Then search for 'MockTool' in pkg/agent. Use get_symbols to map tools_test.go, then update it so the return string 'returns the input' becomes 'returns the raw input'. Use apply_patch for the change. Finally, search for 'raw input' to confirm."
	
	fmt.Printf("[TASK]: %s\n", task)

	// Up to 10 steps for this complex sequence
	currentTask := task
	for i := 1; i <= 10; i++ {
		t.Logf("\n--- STEP %d ---", i)
		res, err := a.Step(ctx, currentTask)
		if err != nil {
			// GRACEFUL SKIP ON LIVE RATE LIMITS
			if strings.Contains(strings.ToLower(err.Error()), "429") {
				t.Skip("Skipping live integration test: Rate limited by OpenRouter (429)")
			}
			t.Fatalf("Step %d failed: %v", i, err)
		}
		currentTask = "" // Clear task after first turn to allow ReAct loop to continue

		t.Logf("[Usage: %d tokens (Total: %d)]", res.Usage.TotalTokens, a.TotalUsage.TotalTokens)

		// Handle Safety Governor approval automatically in the test
		if res.Status == agent.StatusPending {
			t.Logf("[APPROVAL REQUIRED]: %d actions", len(res.ToolCalls))
			res, err = a.Approve(ctx)
			if err != nil {
				if strings.Contains(strings.ToLower(err.Error()), "429") {
					t.Skip("Skipping live integration test: Rate limited by OpenRouter (429)")
				}
				t.Fatalf("Approval failed: %v", err)
			}
		}

		thought := res.Thought

		if strings.Contains(strings.ToLower(thought), "task complete") || 
		   strings.Contains(strings.ToLower(thought), "final answer") {
			t.Log("Task completion detected.")
			break
		}
	}
	
	t.Log("\n--- END OF FULL CYCLE ---")

	// 3. Verification: Read tools_test.go back from the REAL file system
	// Adjust path since we are in test/
	testFilePath := "../pkg/agent/tools_test.go"
	data, err := os.ReadFile(testFilePath)
	if err != nil {
		t.Logf("Warning: tools_test.go not found at %s, trying current dir", testFilePath)
		data, _ = os.ReadFile("tools_test.go")
	}

	if !strings.Contains(string(data), "returns the raw input") {
		t.Errorf("FAIL: File was not updated correctly. Content: %s", string(data))
	} else {
		fmt.Printf("SUCCESS: Verified 'raw input' exists in tools_test.go on disk.\n")
	}

	// REVERT: Change it back so we don't pollute the codebase for the next run
	original := strings.Replace(string(data), "returns the raw input", "returns the input", 1)
	os.WriteFile(testFilePath, []byte(original), 0644)
	os.Remove("../PLAN.md")
}

func TestIntegrationPrivacyShield(t *testing.T) {
	// 1. Load config from armage.json (relative to test/)
	configData, err := os.ReadFile("../configs/armage.json")
	if err != nil {
		t.Skip("Skipping TestIntegrationPrivacyShield: armage.json not found")
	}

	var config struct {
		LocalScrubber struct {
			BinaryPath string `json:"binary_path"`
			ModelPath  string `json:"model_path"`
			URL        string `json:"url"`
		} `json:"local_scrubber"`
	}
	if err := json.Unmarshal(configData, &config); err != nil {
		t.Fatalf("Failed to parse armage.json: %v", err)
	}

	if config.LocalScrubber.BinaryPath == "" || config.LocalScrubber.ModelPath == "" {
		t.Skip("Skipping TestIntegrationPrivacyShield: binary_path or model_path not set in armage.json")
	}

	// 2. Setup LLM with Scrubbing
	innerLLM := &MockMultiStepLLM{
		Responses: []string{
			"Thought: I will echo the secret.\nAction: shell(\"echo 'The REDACTED_KEY is safe'\")",
		},
	}

	scrubber := &provider.LocalScrubber{
		BaseURL:    config.LocalScrubber.URL,
		BinaryPath: config.LocalScrubber.BinaryPath,
		ModelPath:  config.LocalScrubber.ModelPath,
	}
	defer scrubber.Stop() // Ensure cleanup

	sllm := provider.NewScrubbingLLM(innerLLM, scrubber, "")
	reg := agent.NewRegistry()
	reg.Register(&agent.ShellTool{})
	a := agent.New(sllm, reg)
	a.AddSystemPrompt("You are Armage")

	// 3. Run a turn with "sensitive" info
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	task := "My server key is sk-TEST-1234. Please confirm it's safe and redact it properly."
	_, err = a.Step(ctx, task)
	if err != nil {
		t.Fatalf("Integration step failed: %v", err)
	}

	// 4. Verify that the inner LLM received a scrubbed message
	if len(innerLLM.LastMessages) < 2 {
		t.Fatalf("Inner LLM did not receive enough messages")
	}

	lastUserMsg := innerLLM.LastMessages[len(innerLLM.LastMessages)-1].Content
	t.Logf("Last User Msg received by Inner LLM: %s", lastUserMsg)

	if strings.Contains(lastUserMsg, "sk-TEST-1234") {
		t.Errorf("PII Leak! Inner LLM received unscrubbed key: %s", lastUserMsg)
	}
}

func TestIntegrationAutoRetry(t *testing.T) {
	reg := agent.NewRegistry()
	reg.Register(&agent.ShellTool{})

	// Step 1: Call non-existent tool
	// Step 2: React to error and call shell
	llm := &MockMultiStepLLM{
		Responses: []string{
			"Thought: I will use a broken tool.\nAction: broken_tool(\"data\")",
			"Thought: broken_tool failed. I will use shell instead.\nAction: shell(\"whoami\")",
		},
	}

	a := agent.New(llm, reg)
	ctx := context.Background()

	// Turn 1
	res, err := a.Step(ctx, "Start")
	if err != nil {
		t.Fatalf("Step 1 failed: %v", err)
	}

	if len(a.History) < 3 {
		t.Fatalf("History too short, got %d", len(a.History))
	}

	lastMsg := a.History[len(a.History)-1].Content
	if !strings.Contains(lastMsg, "Error: Tool 'broken_tool' not found") {
		t.Errorf("Expected error observation, got: %s", lastMsg)
	}

	// Turn 2: Automatic retry (empty input)
	res, err = a.Step(ctx, "")
	if err != nil {
		t.Fatalf("Step 2 failed: %v", err)
	}

	if len(res.ToolCalls) == 0 || res.ToolCalls[0].Name != "shell" {
		t.Errorf("Expected retry with 'shell', got: %v", res.ToolCalls)
	}
}

// MockMultiStepLLM allows specifying multiple responses for sequential turns.
type MockMultiStepLLM struct {
	Responses    []string
	Turn         int
	LastMessages []provider.Message
}

func (m *MockMultiStepLLM) Chat(ctx context.Context, messages []provider.Message) (string, provider.Usage, error) {
	m.LastMessages = messages
	if m.Turn >= len(m.Responses) {
		return "Final Answer: I am done.", provider.Usage{TotalTokens: 10}, nil
	}
	resp := m.Responses[m.Turn]
	m.Turn++
	return resp, provider.Usage{TotalTokens: 50}, nil
}
