package agent

import (
	"encoding/json"
	"regexp"
	"strings"
)

var (
	thoughtRegex  = regexp.MustCompile(`(?s)Thought:\s*(.*?)(?:\nAction:|\n<tool_call>|$)`)
	actionRegex   = regexp.MustCompile(`Action:\s*(\w+)\((.*?)\)`)
	xmlBlockRegex = regexp.MustCompile(`(?s)<tool_call>(.*?)</tool_call>`)
	xmlFuncRegex  = regexp.MustCompile(`<function=(\w+)>`)
	xmlParamRegex = regexp.MustCompile(`(?s)<parameter=(\w+)>(.*?)</parameter>`)
	jsonBlockRegex = regexp.MustCompile(`(?s)\{.*\}`)
)

// Parse extracts the Thought and all Actions (tool calls) from the LLM's response.
func Parse(input string) (thought string, toolCalls []ToolCall, err error) {
	trimmedInput := strings.TrimSpace(input)

	// 1. Try to extract and parse a JSON block (Flexible JSON handling)
	if jsonMatch := jsonBlockRegex.FindString(trimmedInput); jsonMatch != "" {
		var raw map[string]interface{}
		if err := json.Unmarshal([]byte(jsonMatch), &raw); err == nil {
			// Extract Thought
			if t, ok := raw["thought"].(string); ok { thought = t }
			if t, ok := raw["Thought"].(string); ok { thought = t }

			// Extract Actions (Standard Format)
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

			// Extract Actions (Fuzzy Format: "Action": [{"pattern": "..."}])
			if actRaw, ok := raw["Action"].([]interface{}); ok {
				for _, item := range actRaw {
					if m, ok := item.(map[string]interface{}); ok {
						// Infer tool name from keys
						name := "shell" 
						if _, ok := m["pattern"]; ok { name = "grep_search" }
						if _, ok := m["path"]; ok && name == "shell" { name = "list_dir" }
						
						argsBytes, _ := json.Marshal(m)
						toolCalls = append(toolCalls, ToolCall{Name: name, Args: string(argsBytes)})
					}
				}
			} else if actStr, ok := raw["Action"].(string); ok {
				// Nested ReAct style inside JSON
				_, actionCalls, _ := Parse("Action: " + actStr)
				toolCalls = append(toolCalls, actionCalls...)
			}
		}
		if thought != "" || len(toolCalls) > 0 {
			return stripInstructions(thought), toolCalls, nil
		}
	}

	// 2. Extract Thought using markers
	thoughtMatch := thoughtRegex.FindStringSubmatch(input)
	if len(thoughtMatch) > 1 {
		thought = strings.TrimSpace(thoughtMatch[1])
	}

	// 3. Extract ReAct Actions (Balanced Parentheses)
	lines := strings.Split(input, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Action:") {
			actionPart := strings.TrimSpace(strings.TrimPrefix(line, "Action:"))
			openParenIdx := strings.Index(actionPart, "(")
			if openParenIdx > 0 {
				toolName := strings.TrimSpace(actionPart[:openParenIdx])
				depth, closingParenIdx := 0, -1
				for i := openParenIdx; i < len(actionPart); i++ {
					if actionPart[i] == '(' { depth++ } else if actionPart[i] == ')' {
						depth--
						if depth == 0 { closingParenIdx = i; break }
					}
				}
				if closingParenIdx > openParenIdx {
					args := actionPart[openParenIdx+1 : closingParenIdx]
					toolCalls = append(toolCalls, ToolCall{Name: toolName, Args: strings.TrimSpace(args)})
				}
			}
		}
	}

	// 4. Extract XML Tool Calls
	xmlBlocks := xmlBlockRegex.FindAllStringSubmatch(input, -1)
	for _, block := range xmlBlocks {
		content := block[1]
		funcMatch := xmlFuncRegex.FindStringSubmatch(content)
		if len(funcMatch) < 2 { continue }
		funcName := funcMatch[1]
		params := make(map[string]interface{})
		paramMatches := xmlParamRegex.FindAllStringSubmatch(content, -1)
		for _, pm := range paramMatches {
			if len(pm) > 2 { params[pm[1]] = strings.TrimSpace(pm[2]) }
		}
		argsJSON, _ := json.Marshal(params)
		toolCalls = append(toolCalls, ToolCall{Name: funcName, Args: strings.TrimSpace(string(argsJSON))})
	}

	if thought == "" && len(toolCalls) == 0 { 
		thought = strings.TrimSpace(input) 
	}
	
	return stripInstructions(thought), toolCalls, nil
}

func stripInstructions(text string) string {
	placeholders := []string{
		"[Your detailed reasoning about the current state and next steps]",
		"Action: ToolName([JSON Arguments])",
		"Thought: [Your detailed reasoning",
	}
	clean := text
	for _, p := range placeholders {
		clean = strings.ReplaceAll(clean, p, "")
	}
	return strings.TrimSpace(clean)
}
