package runner

import (
	"os"
	"testing"
	"unicode/utf8"
)

func TestWriteTempPromptSanitizesInvalidUTF8(t *testing.T) {
	dir := t.TempDir()
	path, err := writeTempPrompt(dir, "hello "+string([]byte{0xff, 0xfe})+" world")
	if err != nil {
		t.Fatalf("writeTempPrompt: %v", err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read prompt: %v", err)
	}
	if !utf8.Valid(body) {
		t.Fatalf("prompt file is not valid utf-8: %q", body)
	}
	if got := string(body); got != "hello � world" {
		t.Fatalf("unexpected sanitized prompt: %q", got)
	}
}
