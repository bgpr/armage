package agent

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/user/armage/pkg/provider"
)

func TestIntegrationReAct(t *testing.T) {
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		t.Skip("Skipping integration test: OPENROUTER_API_KEY not set")
	}

	// 1. Setup with a FREE model (Gemma 3 12B)
	model := "google/gemma-3-12b-it"
	llm := provider.NewOpenRouter(apiKey, model)
	reg := NewRegistry()
	reg.Register(&ShellTool{})

	a := New(llm, reg)
	a.AddSystemPrompt(`You are Armage, an expert coding agent for Termux. 
Follow the ReAct pattern strictly: 
Thought: [Your reasoning]
Action: shell("[command]")

Example:
Thought: I need to list files.
Action: shell("ls")
`)

	fmt.Printf("\n--- Agentic ReAct Integration Test (Verbose) ---\n")

	// 2. Ask a simple question that requires a tool
	ctx := context.Background()
	userInput := "List the files in the current directory using the shell tool."
	fmt.Printf("[INPUT]: %s\n", userInput)

	_, err := a.Step(ctx, userInput)
	if err != nil {
		t.Fatalf("Agent step failed: %v", err)
	}

	// 3. Display the Conversation and Verify the loop worked
	foundAction := false
	foundObservation := false

	fmt.Printf("\n--- CONVERSATION LOG ---\n")
	for _, msg := range a.History {
		if msg.Role == "system" {
			continue // Skip system prompt for brevity
		}
		
		role := strings.ToUpper(msg.Role)
		fmt.Printf("[%s]:\n%s\n\n", role, msg.Content)

		if msg.Role == "assistant" && strings.Contains(msg.Content, "Action: shell") {
			foundAction = true
		}
		if msg.Role == "user" && strings.Contains(msg.Content, "Observation:") {
			foundObservation = true
		}
	}
	fmt.Printf("--- END LOG ---\n\n")

	if !foundAction {
		t.Error("FAIL: Agent did not produce a shell action.")
	}
	if !foundObservation {
		t.Error("FAIL: Agent did not receive a tool observation.")
	}
	
	if foundAction && foundObservation {
		fmt.Println("SUCCESS: Agent completed the ReAct loop successfully.")
	}
}
