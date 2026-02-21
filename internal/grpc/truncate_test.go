package grpc

import (
	"fmt"
	"strings"
	"testing"
)

func TestTruncateForLog(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantFull bool
	}{
		{"short string unchanged", `{"id": 1}`, true},
		{"exactly at limit", strings.Repeat("x", maxLogBodyLen), true},
		{"one over limit is truncated", strings.Repeat("x", maxLogBodyLen+1), false},
		{"large string is truncated", strings.Repeat("a", 10000), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncateForLog(tt.input)
			if tt.wantFull {
				if result != tt.input {
					t.Errorf("expected unchanged output, got len %d", len(result))
				}
				return
			}
			// Truncated case
			if !strings.HasPrefix(result, tt.input[:maxLogBodyLen]) {
				t.Error("truncated output should start with the original prefix")
			}
			wantSuffix := fmt.Sprintf("... (%d bytes total)", len(tt.input))
			if !strings.HasSuffix(result, wantSuffix) {
				t.Errorf("expected suffix %q, got %q", wantSuffix, result[maxLogBodyLen:])
			}
		})
	}
}

func TestTruncateForLog_Empty(t *testing.T) {
	if result := truncateForLog(""); result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
}
