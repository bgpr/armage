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

// Parse extracts the Thought and all Actions (tool calls) from the LLM's response.
// It supports ReAct format, standard XML, and pure JSON responses.
func Parse(input string) (thought string, toolCalls []ToolCall, err error) {
	trimmedInput := strings.TrimSpace(input)

	// 1. Try to parse as a pure JSON object (Modern/Structured models)
	if strings.HasPrefix(trimmedInput, "{") && strings.HasSuffix(trimmedInput, "}") {
		var jsonRes struct {
			Thought   string     `json:"thought"`
			Action    string     `json:"action"`
			ToolCalls []ToolCall `json:"tool_calls"`
		}
		if err := json.Unmarshal([]byte(trimmedInput), &jsonRes); err == nil {
			thought = jsonRes.Thought
			if jsonRes.Action != "" {
				// Try to parse the action string (e.g. "tool(args)")
				_, actionCalls, _ := Parse("Action: " + jsonRes.Action)
				toolCalls = append(toolCalls, actionCalls...)
			}
			if len(jsonRes.ToolCalls) > 0 {
				toolCalls = append(toolCalls, jsonRes.ToolCalls...)
			}
			if thought != "" || len(toolCalls) > 0 {
				return thought, toolCalls, nil
			}
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

	// 4. Extract XML Tool Calls
	xmlBlocks := xmlBlockRegex.FindAllStringSubmatch(input, -1)
	for _, block := range xmlBlocks {
		content := block[1]
		funcMatch := xmlFuncRegex.FindStringSubmatch(content)
		if len(funcMatch) < 2 {
			continue
		}
		funcName := funcMatch[1]

		params := make(map[string]interface{})
		paramMatches := xmlParamRegex.FindAllStringSubmatch(content, -1)
		for _, pm := range paramMatches {
			if len(pm) > 2 {
				key := pm[1]
				val := strings.TrimSpace(pm[2])
				params[key] = val
			}
		}

		argsJSON, _ := json.Marshal(params)
		toolCalls = append(toolCalls, ToolCall{
			Name: funcName,
			Args: strings.TrimSpace(string(argsJSON)),
		})
	}

	// 5. Fallback: If no markers but looks like a final answer
	if thought == "" && len(toolCalls) == 0 {
		thought = strings.TrimSpace(input)
	}

	if thought == "" && len(toolCalls) == 0 {
		return "", nil, errors.New("no Thought or Action found in input")
	}

	return thought, toolCalls, nil
}

