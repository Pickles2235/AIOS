package policy

import (
	"fmt"
	"strings"
)

const (
	DefaultLimit   = 20
	MaxCursorBytes = 8192
)

func BoundedLimit(requested, maximum int) (int, error) {
	if requested == 0 {
		requested = min(DefaultLimit, maximum)
	}
	if requested < 1 || requested > maximum {
		return 0, fmt.Errorf("limit must be between 1 and %d", maximum)
	}
	return requested, nil
}
func Cursor(value string) (string, error) {
	if len(value) > MaxCursorBytes {
		return "", fmt.Errorf("cursor exceeds %d bytes", MaxCursorBytes)
	}
	return value, nil
}
func Query(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("query must not be empty")
	}
	if len(value) > 256 {
		return "", fmt.Errorf("query exceeds 256 bytes")
	}
	return value, nil
}
