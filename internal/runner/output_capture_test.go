package runner

import (
	"bytes"
	"testing"
)

func TestBoundedOutputKeepsLatestBytes(t *testing.T) {
	got := newBoundedOutput(5)
	_, _ = got.Write([]byte("1234"))
	_, _ = got.Write([]byte("567"))
	if string(got.Bytes()) != "34567" {
		t.Fatalf("captured output = %q, want %q", got.String(), "34567")
	}
	if !got.Truncated() {
		t.Fatal("expected capture to report truncation")
	}
}

func TestBoundedOutputAcceptsLargeWriteWithoutGrowing(t *testing.T) {
	got := newBoundedOutput(32)
	input := bytes.Repeat([]byte("x"), 1024*1024)
	if n, err := got.Write(input); err != nil || n != len(input) {
		t.Fatalf("Write() = (%d, %v), want (%d, nil)", n, err, len(input))
	}
	if len(got.Bytes()) != 32 || !got.Truncated() {
		t.Fatalf("bounded output size=%d truncated=%v", len(got.Bytes()), got.Truncated())
	}
}
