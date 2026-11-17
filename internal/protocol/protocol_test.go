package protocol_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/Reidevl/rn-lens/internal/protocol"
)

const schemaPath = "../../protocol/schema/lens-protocol.v1.schema.json"

func loadSchema(t *testing.T) *jsonschema.Resolved {
	t.Helper()
	raw, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	var s jsonschema.Schema
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatalf("parse schema: %v", err)
	}
	resolved, err := s.Resolve(nil)
	if err != nil {
		t.Fatalf("resolve schema: %v", err)
	}
	return resolved
}

func fixtures(t *testing.T, dir string) map[string][]byte {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("no fixtures in %s (%v)", dir, err)
	}
	out := map[string][]byte{}
	for _, p := range paths {
		raw, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		out[strings.TrimSuffix(filepath.Base(p), ".json")] = raw
	}
	return out
}

func asInstance(t *testing.T, raw []byte) any {
	t.Helper()
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	return v
}

// Every valid fixture must validate against the schema, decode strictly into
// the Go types (so no schema field lacks a Go counterpart), and still
// validate after a Go round-trip (so no Go field lacks a schema counterpart).
func TestFixturesConformOnBothSides(t *testing.T) {
	t.Parallel()
	schema := loadSchema(t)
	var mu sync.Mutex
	seen := map[string]bool{}

	for name, raw := range fixtures(t, "../../protocol/fixtures") {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if err := schema.Validate(asInstance(t, raw)); err != nil {
				t.Fatalf("schema rejects fixture: %v", err)
			}

			msg, err := protocol.Decode(raw)
			if err != nil {
				t.Fatalf("Decode: %v", err)
			}
			dst := protocol.PayloadType(msg.Type)
			if dst == nil {
				t.Fatalf("no Go payload type for %q", msg.Type)
			}
			if err := msg.PayloadInto(dst); err != nil {
				t.Fatalf("strict payload decode: %v", err)
			}

			again, err := protocol.Encode(msg.Type, msg.ID, dst)
			if err != nil {
				t.Fatalf("Encode: %v", err)
			}
			if err := schema.Validate(asInstance(t, again)); err != nil {
				t.Fatalf("schema rejects the Go round-trip %s: %v", again, err)
			}
			mu.Lock()
			seen[msg.Type] = true
			mu.Unlock()
		})
	}

	t.Cleanup(func() {
		for _, typ := range []string{
			protocol.TypeHello, protocol.TypeWelcome, protocol.TypeSnapshotRequest, protocol.TypeQueriesSnapshot,
			protocol.TypeQueryEvent, protocol.TypeMutationEvent, protocol.TypeOnlineStatus, protocol.TypeQueryAction,
			protocol.TypeActionResult, protocol.TypeOnlineSet, protocol.TypePing, protocol.TypePong,
		} {
			if !seen[typ] {
				t.Errorf("no fixture covers message type %q", typ)
			}
		}
	})
}

func TestInvalidFixturesAreRejected(t *testing.T) {
	t.Parallel()
	schema := loadSchema(t)
	for name, raw := range fixtures(t, "../../protocol/fixtures/invalid") {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if err := schema.Validate(asInstance(t, raw)); err == nil {
				t.Fatal("schema accepted an invalid fixture")
			}
		})
	}
}

func TestDecodeRejectsOtherMajorsAndUnknownFields(t *testing.T) {
	t.Parallel()
	if _, err := protocol.Decode([]byte(`{"v":2,"type":"ping","payload":{}}`)); !errors.Is(err, protocol.ErrVersion) {
		t.Fatalf("err = %v, want ErrVersion", err)
	}
	msg, err := protocol.Decode([]byte(`{"v":1,"type":"online.set","payload":{"online":true,"extra":1}}`))
	if err != nil {
		t.Fatal(err)
	}
	var p protocol.OnlineSet
	if err := msg.PayloadInto(&p); err == nil {
		t.Fatal("unknown payload field was accepted")
	}
	if _, err := protocol.Decode([]byte(`{"v":1,"payload":{}}`)); err == nil {
		t.Fatal("missing type was accepted")
	}
}

func TestDeltaDistinguishesUnsetFromZero(t *testing.T) {
	t.Parallel()
	msg, err := protocol.Decode([]byte(`{"v":1,"type":"query.event","payload":{"kind":"updated","queryHash":"h","action":"invalidate","delta":{"isInvalidated":false}}}`))
	if err != nil {
		t.Fatal(err)
	}
	var ev protocol.QueryEvent
	if err := msg.PayloadInto(&ev); err != nil {
		t.Fatal(err)
	}
	if ev.Delta == nil || ev.Delta.IsInvalidated == nil || *ev.Delta.IsInvalidated || ev.Delta.Status != nil {
		t.Fatalf("delta = %+v", ev.Delta)
	}
}
