package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
)

// ApplyPatchTool applies a unified diff to a file.
type ApplyPatchTool struct{}

type patchArgs struct {
	Path  string `json:"path"`
	Patch string `json:"patch"`
}

func (p *ApplyPatchTool) Name() string { return "apply_patch" }

func (p *ApplyPatchTool) Description() string {
	return "Applies a standard unified diff (patch) to a file. Robust for complex multi-line changes."
}

func (p *ApplyPatchTool) Execute(ctx context.Context, args string) (string, error) {
	var a patchArgs
	if err := json.Unmarshal([]byte(args), &a); err != nil {
		return "", fmt.Errorf("invalid JSON for patch: %w", err)
	}

	if a.Path == "" || a.Patch == "" {
		return "", fmt.Errorf("path and patch are required")
	}

	// 1. Write the patch to a temporary file
	patchFile := a.Path + ".patch"
	if err := os.WriteFile(patchFile, []byte(a.Patch), 0644); err != nil {
		return "", fmt.Errorf("failed to write temporary patch file: %w", err)
	}
	defer os.Remove(patchFile)

	// 2. Run the patch command
	// -p0 means don't strip any path components from the filenames in the patch
	cmd := exec.CommandContext(ctx, "patch", "-p0", "-i", patchFile)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("patch command failed: %w", err)
	}

	return fmt.Sprintf("Successfully applied patch to %s:\n%s", a.Path, string(output)), nil
}
