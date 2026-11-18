package sdkws

import (
	"testing"
	"time"
)

// Sessions must list sessions in a deterministic order even when their
// connectedAt timestamps tie; the acceptance sequence breaks the tie.
func TestSessionsOrderIsDeterministicWhenConnectedAtTies(t *testing.T) {
	t.Parallel()
	at := time.Now()
	srv := New(Options{}, nil)
	for i, dev := range []string{"dev-c", "dev-a", "dev-b"} {
		key := SessionKey{AppID: "com.example.app", DeviceID: dev}
		srv.sessions[key] = &Session{Key: key, connectedAt: at, seq: uint64(i + 1)}
	}
	for range 50 {
		got := srv.Sessions()
		if len(got) != 3 {
			t.Fatalf("sessions = %d, want 3", len(got))
		}
		for i, sess := range got {
			if sess.seq != uint64(i+1) {
				t.Fatalf("sessions out of order: %s %s %s", got[0].Key.DeviceID, got[1].Key.DeviceID, got[2].Key.DeviceID)
			}
		}
	}
}
