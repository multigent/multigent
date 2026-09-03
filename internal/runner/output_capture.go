package runner

// maxCapturedOutputBytes bounds the copy kept in memory for parsing session
// IDs, sentinels, and error tails. The complete stream is still written to the
// run log file by the caller.
const maxCapturedOutputBytes = 8 << 20

// boundedOutput keeps only the newest bytes of a process response. Agent CLI
// output is streamed to disk separately, so retaining an unbounded copy here
// creates a memory exhaustion path when a tool is noisy or stuck.
type boundedOutput struct {
	data      []byte
	start     int
	size      int
	truncated bool
}

func newBoundedOutput(limit int) *boundedOutput {
	if limit <= 0 {
		limit = maxCapturedOutputBytes
	}
	return &boundedOutput{data: make([]byte, limit)}
}

func (b *boundedOutput) Write(p []byte) (int, error) {
	if b == nil || len(p) == 0 {
		return len(p), nil
	}
	if len(b.data) == 0 {
		return len(p), nil
	}
	if len(p) >= len(b.data) {
		copy(b.data, p[len(p)-len(b.data):])
		b.start = 0
		b.size = len(b.data)
		b.truncated = true
		return len(p), nil
	}

	if b.size+len(p) > len(b.data) {
		drop := b.size + len(p) - len(b.data)
		b.start = (b.start + drop) % len(b.data)
		b.size -= drop
		b.truncated = true
	}
	writeAt := (b.start + b.size) % len(b.data)
	first := len(p)
	if first > len(b.data)-writeAt {
		first = len(b.data) - writeAt
	}
	copy(b.data[writeAt:writeAt+first], p[:first])
	copy(b.data[:len(p)-first], p[first:])
	b.size += len(p)
	return len(p), nil
}

func (b *boundedOutput) Bytes() []byte {
	if b == nil || b.size == 0 {
		return nil
	}
	out := make([]byte, b.size)
	first := b.size
	if first > len(b.data)-b.start {
		first = len(b.data) - b.start
	}
	copy(out, b.data[b.start:b.start+first])
	copy(out[first:], b.data[:b.size-first])
	return out
}

func (b *boundedOutput) String() string {
	return string(b.Bytes())
}

func (b *boundedOutput) Truncated() bool {
	return b != nil && b.truncated
}
