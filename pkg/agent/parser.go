package agent

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

var (
	thoughtRegex  = regexp.MustCompile(`(?s)Thought:\s*(.*?)(?:\nAction:|\n<tool_call>|$)`)
	xmlBlockRegex = regexp.MustCompile(`(?s)<tool_call>(.*?)</tool_call>`)
	xmlFuncRegex  = regexp.MustCompile(`<function=(\w+)>`)
	xmlParamRegex = regexp.MustCompile(`(?s)<parameter=(\w+)>(.*?)</parameter>`)
)

// Parse extracts the Thought and all Actions (tool calls) from the LLM's response.
func Parse(input string) (thought string, toolCalls []ToolCall, err error) {
	trimmedInput := strings.TrimSpace(input)

	// 1. EXTRACT BALANCED JSON BLOCKS
	for i := 0; i < len(trimmedInput); i++ {
		if trimmedInput[i] == '{' {
			depth := 0
			end := -1
			for j := i; j < len(trimmedInput); j++ {
				if trimmedInput[j] == '{' {
					depth++
				} else if trimmedInput[j] == '}' {
					depth--
					if depth == 0 {
						end = j
						break
					}
				}
			}
			if end != -1 {
				jsonBlock := trimmedInput[i : end+1]
				var raw map[string]interface{}
				if err := json.Unmarshal([]byte(jsonBlock), &raw); err == nil {
					// Extract Thought if present
					if t, ok := raw["thought"].(string); ok {
						thought += t + " "
					}
					if t, ok := raw["Thought"].(string); ok {
						thought += t + " "
					}

					// Standard Tool Calls (OpenAI/OpenRouter format)
					if tcRaw, ok := raw["tool_calls"].([]interface{}); ok {
						for _, item := range tcRaw {
							if m, ok := item.(map[string]interface{}); ok {
								name, _ := m["name"].(string)
								args, _ := m["args"].(string)
								if name != "" {
									toolCalls = append(toolCalls, ToolCall{Name: name, Args: args})
								}
							}
						}
					}

					// SOTA: Prefix-Agnostic Parsing
					// If the block has "command", "path", "pattern", etc., but NO tool name, infer it.
					if _, ok := raw["command"]; ok {
						toolCalls = append(toolCalls, ToolCall{Name: "shell", Args: jsonBlock})
					} else if p, ok := raw["path"].(string); ok {
						name := "list_dir"
						if _, ok := raw["pattern"]; ok {
							name = "grep_search"
						} else if _, ok := raw["content"]; ok {
							name = "write_file"
						} else if strings.HasSuffix(p, ".go") || strings.HasSuffix(p, ".py") {
							// If they just send a path to a code file, assume read or symbols
							name = "read_file"
						}
						toolCalls = append(toolCalls, ToolCall{Name: name, Args: jsonBlock})
					} else if _, ok := raw["action"]; ok {
						// Likely a propose_plan call
						toolCalls = append(toolCalls, ToolCall{Name: "propose_plan", Args: jsonBlock})
					}

					// Handle "Action" as a key (Nemotron style)
					if actRaw, ok := raw["Action"].(string); ok {
						_, innerCalls, _ := Parse("Action: " + actRaw)
						toolCalls = append(toolCalls, innerCalls...)
					}
				}
				i = end
			}
		}
	}

	// 2. EXTRACT RE-ACT ACTIONS (Fallback)
	lines := strings.Split(input, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "Action:") {
			idx := strings.Index(line, "Action:")
			actionPart := strings.TrimSpace(line[idx+7:])
			openParenIdx := strings.Index(actionPart, "(")
			if openParenIdx > 0 {
				toolName := strings.TrimSpace(actionPart[:openParenIdx])
				depth, closingParenIdx := 0, -1
				for i := openParenIdx; i < len(actionPart); i++ {
					if actionPart[i] == '(' {
						depth++
					} else if actionPart[i] == ')' {
						depth--
						if depth == 0 {
							closingParenIdx = i
							break
						}
					}
				}
				if closingParenIdx > openParenIdx {
					args := actionPart[openParenIdx+1 : closingParenIdx]
					// Prevent duplicates if already parsed via JSON
					alreadyParsed := false
					for _, existing := range toolCalls {
						if existing.Name == toolName && existing.Args == strings.TrimSpace(args) {
							alreadyParsed = true
							break
						}
					}
					if !alreadyParsed {
						toolCalls = append(toolCalls, ToolCall{Name: toolName, Args: strings.TrimSpace(args)})
					}
				}
			}
		}
	}

	// 3. EXTRACT XML ACTIONS
	xmlBlocks := xmlBlockRegex.FindAllStringSubmatch(input, -1)
	for _, block := range xmlBlocks {
		content := block[1]
		funcMatch := xmlFuncRegex.FindStringSubmatch(content)
		if len(funcMatch) >= 2 {
			funcName := funcMatch[1]
			params := make(map[string]interface{})
			paramMatches := xmlParamRegex.FindAllStringSubmatch(content, -1)
			for _, pm := range paramMatches {
				if len(pm) > 2 {
					params[pm[1]] = strings.TrimSpace(pm[2])
				}
			}
			argsJSON, _ := json.Marshal(params)
			toolCalls = append(toolCalls, ToolCall{Name: funcName, Args: strings.TrimSpace(string(argsJSON))})
		}
	}

	// 4. EXTRACT THOUGHT USING REGEX (If not found in JSON)
	if thought == "" {
		thoughtMatch := thoughtRegex.FindStringSubmatch(input)
		if len(thoughtMatch) > 1 {
			thought = strings.TrimSpace(thoughtMatch[1])
		}
	}

	// 5. DETECT MALFORMED ATTEMPTS (Self-Correction Trigger)
	if len(toolCalls) == 0 {
		lower := strings.ToLower(input)
		if strings.Contains(lower, "action") || strings.Contains(lower, "tool_call") || (strings.Contains(lower, "{") && strings.Contains(lower, "}")) {
			// Only trigger error if we actually suspect they TRIED to call a tool
			if strings.Contains(lower, "command") || strings.Contains(lower, "path") || strings.Contains(lower, "(") {
				return stripInstructions(strings.TrimSpace(thought)), nil, fmt.Errorf("malformed action detected")
			}
		}
	}

	if thought == "" && len(toolCalls) == 0 {
		thought = input
	}

	return stripInstructions(strings.TrimSpace(thought)), toolCalls, nil
}

func stripInstructions(text string) string {
	placeholders := []string{
		"[Your detailed reasoning about the current state and next steps]",
		"Action: ToolName([JSON Arguments])",
		"Thought: [Your detailed reasoning",
		"Thought: ",
		"```json", "```",
	}
	clean := text
	for _, p := range placeholders {
		clean = strings.ReplaceAll(clean, p, "")
	}
	return strings.TrimSpace(clean)
}
