package telemetry

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
)

// StreamUsage holds the last aggregate usage from Claude stream-json "result" lines
// in a run log or combined stdout buffer.
type StreamUsage struct {
	SawResult       bool // a line with type "result" was seen (tokens/cost are API-reported)
	InputTokens     int64
	OutputTokens    int64
	CacheReadTokens int64
	TotalCostUSD    float64
	HasCost         bool
}

// streamResultUsage handles both Claude (snake_case) and Cursor (camelCase) field names.
type streamResultUsage struct {
	// Claude Code format
	InputTokens           int64 `json:"input_tokens"`
	OutputTokens          int64 `json:"output_tokens"`
	CacheReadInputTokens  int64 `json:"cache_read_input_tokens"`
	CachedInputTokens     int64 `json:"cached_input_tokens"`
	ReasoningOutputTokens int64 `json:"reasoning_output_tokens"`
	// Cursor format
	InputTokensCC  int64 `json:"inputTokens"`
	OutputTokensCC int64 `json:"outputTokens"`
	CacheReadCC    int64 `json:"cacheReadTokens"`
}

func (u streamResultUsage) input() int64  { return coalesce(u.InputTokens, u.InputTokensCC) }
func (u streamResultUsage) output() int64 { return coalesce(u.OutputTokens, u.OutputTokensCC) }
func (u streamResultUsage) cache() int64 {
	return coalesce(coalesce(u.CacheReadInputTokens, u.CachedInputTokens), u.CacheReadCC)
}

func coalesce(a, b int64) int64 {
	if a != 0 {
		return a
	}
	return b
}

type streamResultLine struct {
	Type         string            `json:"type"`
	TotalCostUSD *float64          `json:"total_cost_usd"`
	Usage        streamResultUsage `json:"usage"`
}

// ParseStreamJSONUsage scans newline-delimited JSON (e.g. Claude/Cursor --output-format stream-json).
// The final line with type "result" wins (same semantics as task tokens parsing).
func ParseStreamJSONUsage(data []byte) StreamUsage {
	var out StreamUsage
	for _, line := range bytes.Split(data, []byte{'\n'}) {
		line = bytes.TrimSpace(line)
		if len(line) < 10 {
			continue
		}
		var rl streamResultLine
		if json.Unmarshal(line, &rl) != nil {
			continue
		}
		if rl.Type == "turn.completed" {
			out.SawResult = true
			out.InputTokens = rl.Usage.input()
			out.OutputTokens = rl.Usage.output()
			out.CacheReadTokens = 0
			continue
		}
		if rl.Type == "result" {
			out.SawResult = true
			out.InputTokens = rl.Usage.input()
			out.OutputTokens = rl.Usage.output()
			out.CacheReadTokens = rl.Usage.cache()
			if rl.TotalCostUSD != nil {
				out.TotalCostUSD = *rl.TotalCostUSD
				out.HasCost = true
			}
		}
	}
	if !out.SawResult {
		out = ParseCodexTextUsage(data)
	}
	return out
}

// ParseCodexTextUsage scans Codex CLI text transcripts such as:
//
//	tokens used
//	3,510
//
// Codex currently reports a total token count in this text output, not an
// input/output split. Store it as InputTokens so existing total-token UI can
// still show the actual run consumption.
func ParseCodexTextUsage(data []byte) StreamUsage {
	lines := strings.Split(string(data), "\n")
	for i, raw := range lines {
		line := strings.TrimSpace(strings.TrimPrefix(raw, "[raw]"))
		if strings.EqualFold(line, "tokens used") {
			if n := parseNextTokenCount(lines[i+1:]); n > 0 {
				return StreamUsage{SawResult: true, InputTokens: n}
			}
			continue
		}
		lower := strings.ToLower(line)
		if strings.HasPrefix(lower, "tokens used:") {
			if n := parseTokenCount(strings.TrimSpace(line[len("tokens used:"):])); n > 0 {
				return StreamUsage{SawResult: true, InputTokens: n}
			}
		}
	}
	return StreamUsage{}
}

func parseNextTokenCount(lines []string) int64 {
	for _, raw := range lines {
		line := strings.TrimSpace(strings.TrimPrefix(raw, "[raw]"))
		if line == "" {
			continue
		}
		return parseTokenCount(line)
	}
	return 0
}

func parseTokenCount(s string) int64 {
	s = strings.TrimSpace(strings.TrimSuffix(s, "."))
	s = strings.ReplaceAll(s, ",", "")
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n < 0 {
		return 0
	}
	return n
}
