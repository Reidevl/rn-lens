package cdp_test

import (
	"testing"

	"github.com/Reidevl/rn-lens/internal/platform/cdp"
)

func TestCloseReasonString(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		reason cdp.CloseReason
		want   string
	}{
		{
			name:   "code and message",
			reason: cdp.CloseReason{Status: 1000, Code: cdp.CloseNewDebuggerOpened, Message: "New debugger opened"},
			want:   "[NEW_DEBUGGER_OPENED] New debugger opened (status 1000)",
		},
		{
			name:   "code only",
			reason: cdp.CloseReason{Status: 1000, Code: cdp.ClosePageRemoved},
			want:   "[PAGE_REMOVED] (status 1000)",
		},
		{
			name:   "message only",
			reason: cdp.CloseReason{Status: -1, Message: "failed to read frame header: EOF"},
			want:   "failed to read frame header: EOF (status -1)",
		},
		{
			name:   "neither",
			reason: cdp.CloseReason{Status: 1001},
			want:   "closed (status 1001)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.reason.String(); got != tt.want {
				t.Fatalf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}
