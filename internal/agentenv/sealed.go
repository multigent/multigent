package agentenv

import (
	"fmt"

	"github.com/multigent/multigent/internal/secretbox"
)

// Seal returns a copy whose values are sealed for at-rest storage.
// Existing sealed values are preserved so unrelated config updates do not
// rotate ciphertext or require callers to have plaintext values.
func Seal(values map[string]string) (map[string]string, error) {
	if values == nil {
		return nil, nil
	}
	out := make(map[string]string, len(values))
	for key, value := range values {
		if secretbox.IsSealed(value) || value == "" {
			out[key] = value
			continue
		}
		sealed, err := secretbox.SealString(value)
		if err != nil {
			return nil, fmt.Errorf("seal agent env %q: %w", key, err)
		}
		out[key] = sealed
	}
	return out, nil
}

// Open returns a plaintext copy for runtime injection. Plaintext values remain
// supported so existing databases can be migrated by the next write.
func Open(values map[string]string) (map[string]string, error) {
	if values == nil {
		return nil, nil
	}
	out := make(map[string]string, len(values))
	for key, value := range values {
		if !secretbox.IsSealed(value) {
			out[key] = value
			continue
		}
		opened, err := secretbox.OpenString(value)
		if err != nil {
			return nil, fmt.Errorf("open agent env %q: %w", key, err)
		}
		out[key] = opened
	}
	return out, nil
}
