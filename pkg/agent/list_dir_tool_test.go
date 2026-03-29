package agent

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestListDirTool(t *testing.T) {
	tmpDir := "test_listdir"
	os.MkdirAll(tmpDir+"/subdir", 0755)
	defer os.RemoveAll(tmpDir)
	os.WriteFile(tmpDir+"/file1.txt", []byte("test"), 0644)

	tool := &ListDirTool{}

	t.Run("RawString", func(t *testing.T) {
		res, err := tool.Execute(context.Background(), "test_listdir")
		if err != nil {
			t.Fatalf("Failed: %v", err)
		}
		if !strings.Contains(res, "file1.txt") {
			t.Errorf("Expected file1.txt, got: %s", res)
		}
	})

	t.Run("JSON", func(t *testing.T) {
		res, err := tool.Execute(context.Background(), `{"path": "test_listdir", "depth": 1}`)
		if err != nil {
			t.Fatalf("Failed: %v", err)
		}
		if !strings.Contains(res, "subdir/") {
			t.Errorf("Expected subdir/, got: %s", res)
		}
	})
}
