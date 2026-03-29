package agent

import (
	"encoding/json"
	"errors"
	"regexp"
	"strings"
)

var (
	thoughtRegex = regexp.MustCompile(`(?s)Thought:\s*(.*?)(?:\nAction:|\n<tool_call>|$)`)
	actionRegex  = regexp.MustCompile(`Action:\s*(\w+)\((.*?)\)`)
	// Broader XML match to catch the whole block
	xmlBlockRegex = regexp.MustCompile(`(?s)<tool_call>(.*?)</tool_call>`)
	xmlFuncRegex  = regexp.MustCompile(`<function=(\w+)>`)
	xmlParamRegex = regexp.MustCompile(`(?s)<parameter=(\w+)>(.*?)</parameter>`)
)

// ToolCall represents a single tool invocation request.
type ToolCall struct {
	Name string `json:"name"`
	Args string `json:"args"`
}

// Parse extracts the Thought and all Actions (tool calls) from the LLM's response.
// It supports ReAct format (with balanced parentheses) and standard XML tool calls.
func Parse(input string) (thought string, toolCalls []ToolCall, err error) {
	// 1. Extract Thought
	thoughtMatch := thoughtRegex.FindStringSubmatch(input)
	if len(thoughtMatch) > 1 {
		thought = strings.TrimSpace(thoughtMatch[1])
	}

	// 2. Extract ReAct Actions (Balanced Parentheses)
	// We look for "Action: Name(" and then scan for the matching closing ")"
	lines := strings.Split(input, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Action:") {
			actionPart := strings.TrimPrefix(line, "Action:")
			actionPart = strings.TrimSpace(actionPart)
			
			openParenIdx := strings.Index(actionPart, "(")
			if openParenIdx > 0 {
				toolName := strings.TrimSpace(actionPart[:openParenIdx])
				
				// Scan for balanced closing paren
				depth := 0
				closingParenIdx := -1
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
					toolCalls = append(toolCalls, ToolCall{
						Name: toolName,
						Args: strings.TrimSpace(args),
					})
				}
			}
		}
	}

	// 3. Extract XML Tool Calls (Multi-parameter support)
	xmlBlocks := xmlBlockRegex.FindAllStringSubmatch(input, -1)
	for _, block := range xmlBlocks {
		content := block[1]
		
		// Find Function Name
		funcMatch := xmlFuncRegex.FindStringSubmatch(content)
		if len(funcMatch) < 2 {
			continue
		}
		funcName := funcMatch[1]

		// Extract all Parameters
		params := make(map[string]interface{})
		paramMatches := xmlParamRegex.FindAllStringSubmatch(content, -1)
		for _, pm := range paramMatches {
			if len(pm) > 2 {
				key := pm[1]
				val := strings.TrimSpace(pm[2])
				params[key] = val
			}
		}

		// Reconstruct as JSON string
		argsJSON, _ := json.Marshal(params)
		toolCalls = append(toolCalls, ToolCall{
			Name: funcName,
			Args: strings.TrimSpace(string(argsJSON)),
		})
	}

	// 4. Fallback: If no markers but looks like tool usage (e.g. no Thought tag)
	if thought == "" && len(toolCalls) == 0 {
		thought = strings.TrimSpace(input)
	}

	if thought == "" && len(toolCalls) == 0 {
		return "", nil, errors.New("no Thought or Action found in input")
	}

	return thought, toolCalls, nil
}
