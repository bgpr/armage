package agent

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestApplyPatchTool(t *testing.T) {
	tmpFile := "test_patch.txt"
	content := "Line 1\nLine 2\nLine 3"
	os.WriteFile(tmpFile, []byte(content), 0644)
	defer os.Remove(tmpFile)

	tool := &ApplyPatchTool{}

	t.Run("ValidPatch", func(t *testing.T) {
		patch := "--- test_patch.txt\n+++ test_patch.txt\n@@ -1,3 +1,3 @@\n Line 1\n-Line 2\n+Updated 2\n Line 3"
		_, err := tool.Execute(context.Background(), `{"path": "test_patch.txt", "patch": "`+strings.ReplaceAll(patch, "\n", "\\n")+`"}`)
		if err != nil {
			t.Fatalf("Patch failed: %v", err)
		}
		updated, _ := os.ReadFile(tmpFile)
		if !strings.Contains(string(updated), "Updated 2") {
			t.Errorf("File not patched correctly")
		}
	})

	t.Run("InvalidPatch", func(t *testing.T) {
		_, err := tool.Execute(context.Background(), `{"path": "test_patch.txt", "patch": "invalid"}`)
		if err == nil {
			t.Errorf("Expected error for invalid patch")
		}
	})

	t.Run("MissingFile", func(t *testing.T) {
		_, err := tool.Execute(context.Background(), `{"path": "missing.txt", "patch": "--- missing.txt\n+++ missing.txt\n@@ -0,0 +1 @@\n+new"}`)
		if err == nil {
			t.Errorf("Expected error for missing file")
		}
	})
}
