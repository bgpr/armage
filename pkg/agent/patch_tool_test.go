package agent

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestApplyPatchTool(t *testing.T) {
	// 1. Setup a mock file
	tmpFile := "test_patch.txt"
	content := `line 1
line 2
line 3
`
	os.WriteFile(tmpFile, []byte(content), 0644)
	defer os.Remove(tmpFile)

	tool := &ApplyPatchTool{}

	// 2. Define a unified diff patch
	patch := `--- test_patch.txt
+++ test_patch.txt
@@ -1,3 +1,3 @@
 line 1
-line 2
+line 2 modified
 line 3
`

	// 3. Execute patch application
	res, err := tool.Execute(context.Background(), `{"path": "test_patch.txt", "patch": "`+strings.ReplaceAll(patch, "\n", "\\n")+`"}`)
	if err != nil {
		t.Fatalf("ApplyPatchTool failed: %v", err)
	}

	if !strings.Contains(res, "Successfully applied patch") {
		t.Errorf("Expected success message, got: %s", res)
	}

	// 4. Verify file content
	newContent, _ := os.ReadFile(tmpFile)
	expected := `line 1
line 2 modified
line 3
`
	if string(newContent) != expected {
		t.Errorf("File content mismatch.\nExpected:\n%s\nGot:\n%s", expected, string(newContent))
	}
}
