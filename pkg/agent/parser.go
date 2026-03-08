package agent

import (
	"errors"
	"regexp"
	"strings"
)

var (
	thoughtRegex = regexp.MustCompile(`(?s)Thought:\s*(.*?)(?:\nAction:|$)`)
	actionRegex  = regexp.MustCompile(`Action:\s*(\w+)\((.*?)\)`)
)

// Parse extracts the Thought and Action (tool call) from the LLM's response.
func Parse(input string) (thought, tool, args string, err error) {
	// Extract Thought
	thoughtMatch := thoughtRegex.FindStringSubmatch(input)
	if len(thoughtMatch) > 1 {
		thought = strings.TrimSpace(thoughtMatch[1])
	}

	// Extract Action (Tool and Arguments)
	actionMatch := actionRegex.FindStringSubmatch(input)
	if len(actionMatch) > 2 {
		tool = strings.TrimSpace(actionMatch[1])
		args = strings.TrimSpace(actionMatch[2])
	}

	if thought == "" && tool == "" {
		return "", "", "", errors.New("no Thought or Action found in input")
	}

	return thought, tool, args, nil
}
