package api

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestReadFileTailLimitsLargeLog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "run.log")
	body := append(bytes.Repeat([]byte("a"), 1024), []byte("tail-content")...)
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatalf("write log: %v", err)
	}

	data, truncated, err := readFileTail(path, 32)
	if err != nil {
		t.Fatalf("read tail: %v", err)
	}
	if !truncated {
		t.Fatal("expected large log to be marked truncated")
	}
	if len(data) != 32 {
		t.Fatalf("tail length = %d, want 32", len(data))
	}
	if !bytes.HasSuffix(data, []byte("tail-content")) {
		t.Fatalf("tail did not include latest content: %q", string(data))
	}
}

func TestReadFileTailReadsSmallLog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "run.log")
	if err := os.WriteFile(path, []byte("small"), 0o600); err != nil {
		t.Fatalf("write log: %v", err)
	}

	data, truncated, err := readFileTail(path, 32)
	if err != nil {
		t.Fatalf("read tail: %v", err)
	}
	if truncated {
		t.Fatal("small log should not be marked truncated")
	}
	if string(data) != "small" {
		t.Fatalf("data = %q", string(data))
	}
}
