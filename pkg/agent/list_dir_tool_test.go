package agent

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestListDirTool(t *testing.T) {
	// 1. Setup mock directory structure
	tmpDir := "test_listdir"
	os.MkdirAll(tmpDir+"/subdir/subsubdir", 0755)
	defer os.RemoveAll(tmpDir)

	os.WriteFile(tmpDir+"/file1.txt", []byte("test"), 0644)
	os.WriteFile(tmpDir+"/subdir/file2.txt", []byte("test"), 0644)
	os.WriteFile(tmpDir+"/subdir/subsubdir/file3.txt", []byte("test"), 0644)

	tool := &ListDirTool{}

	// Test Case 1: Depth 0 (Current directory only)
	res, err := tool.Execute(context.Background(), `{"path": "test_listdir", "depth": 0}`)
	if err != nil {
		t.Fatalf("ListDir failed: %v", err)
	}
	if !strings.Contains(res, "file1.txt") || !strings.Contains(res, "subdir/") {
		t.Errorf("Depth 0 should contain file1 and subdir, got:\n%s", res)
	}
	if strings.Contains(res, "file2.txt") {
		t.Errorf("Depth 0 should NOT contain nested file2.txt, got:\n%s", res)
	}

	// Test Case 2: Depth 1 (One level deep)
	res, err = tool.Execute(context.Background(), `{"path": "test_listdir", "depth": 1}`)
	if err != nil {
		t.Fatalf("ListDir failed: %v", err)
	}
	// We expect subdir/ to be followed by indented file2.txt
	if !strings.Contains(res, "  file2.txt") {
		t.Errorf("Depth 1 should contain indented file2.txt, got:\n%s", res)
	}
	if strings.Contains(res, "file3.txt") {
		t.Errorf("Depth 1 should NOT contain sub-sub-nested file3.txt, got:\n%s", res)
	}
}
