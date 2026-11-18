package protocol_test

import (
	"encoding/json"
	"errors"
	"os"
	"path"
	"path/filepath"
	"slices"
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

			msg, err := protocol.DecodeStrict(raw)
			if err != nil {
				t.Fatalf("DecodeStrict: %v", err)
			}
			dst := protocol.PayloadType(msg.Type)
			if dst == nil {
				t.Fatalf("no Go payload type for %q", msg.Type)
			}
			if err := msg.PayloadIntoStrict(dst); err != nil {
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
			protocol.TypeStorageSnapshot, protocol.TypeStorageEvent, protocol.TypeStorageAction, protocol.TypeStorageResult,
			protocol.TypePluginRegister, protocol.TypePluginView, protocol.TypePluginAction, protocol.TypePluginResult,
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

func TestDecodeRejectsOtherMajorsAndMissingType(t *testing.T) {
	t.Parallel()
	if _, err := protocol.Decode([]byte(`{"v":2,"type":"ping","payload":{}}`)); !errors.Is(err, protocol.ErrVersion) {
		t.Fatalf("err = %v, want ErrVersion", err)
	}
	if _, err := protocol.Decode([]byte(`{"v":1,"payload":{}}`)); err == nil {
		t.Fatal("missing type was accepted")
	}
	if _, err := protocol.Decode([]byte(`{"v":1,"type":"ping","payload":{}} trailing`)); err == nil {
		t.Fatal("trailing data was accepted")
	}
}

// Minors are additive: a decoder must ignore fields it does not know so an
// older Lens can talk to a newer SDK (protocol/README.md). The strict variants
// exist for the fixture conformance test only.
func TestDecodeIgnoresUnknownFieldsUnlessStrict(t *testing.T) {
	t.Parallel()
	raw := []byte(`{"v":1,"type":"online.set","future":true,"payload":{"online":true,"extra":1}}`)

	msg, err := protocol.Decode(raw)
	if err != nil {
		t.Fatalf("Decode with unknown envelope field: %v", err)
	}
	var p protocol.OnlineSet
	if err := msg.PayloadInto(&p); err != nil || !p.Online {
		t.Fatalf("PayloadInto with unknown payload field: err=%v online=%v", err, p.Online)
	}

	if _, err := protocol.DecodeStrict(raw); err == nil {
		t.Fatal("DecodeStrict accepted an unknown envelope field")
	}
	strictMsg, err := protocol.DecodeStrict([]byte(`{"v":1,"type":"online.set","payload":{"online":true,"extra":1}}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := strictMsg.PayloadIntoStrict(&p); err == nil {
		t.Fatal("PayloadIntoStrict accepted an unknown payload field")
	}
}

func TestIsActionTypeAcceptsExactlyTheSchemaActions(t *testing.T) {
	t.Parallel()
	for _, typ := range protocol.ActionTypes {
		if !protocol.IsActionType(typ) {
			t.Fatalf("IsActionType(%q) = false, want true", typ)
		}
	}
	for _, typ := range []string{"", "refetch", "EXPLODE", " REFETCH"} {
		if protocol.IsActionType(typ) {
			t.Fatalf("IsActionType(%q) = true, want false", typ)
		}
	}
}

func TestIsStorageActionTypeAcceptsExactlyTheSchemaActions(t *testing.T) {
	t.Parallel()
	for _, typ := range protocol.StorageActionTypes {
		if !protocol.IsStorageActionType(typ) {
			t.Fatalf("IsStorageActionType(%q) = false, want true", typ)
		}
	}
	for _, typ := range []string{"", "get", "REFETCH", "DROP"} {
		if protocol.IsStorageActionType(typ) {
			t.Fatalf("IsStorageActionType(%q) = true, want false", typ)
		}
	}
}

func TestPluginBlockTypesMatchTheSchema(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatal(err)
	}
	var schema struct {
		Defs map[string]struct {
			OneOf []struct {
				Ref string `json:"$ref"`
			} `json:"oneOf"`
			Properties struct {
				Type struct{ Const string } `json:"type"`
			} `json:"properties"`
		} `json:"$defs"`
	}
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatal(err)
	}
	kinds := make([]string, 0, len(protocol.PluginBlockTypes))
	for _, ref := range schema.Defs["PluginBlock"].OneOf {
		kinds = append(kinds, schema.Defs[path.Base(ref.Ref)].Properties.Type.Const)
	}
	if !slices.Equal(kinds, protocol.PluginBlockTypes) {
		t.Fatalf("PluginBlockTypes = %v, schema has %v", protocol.PluginBlockTypes, kinds)
	}
}

func TestStorageValueRoundTripsBytesAsBase64(t *testing.T) {
	t.Parallel()
	msg, err := protocol.Decode([]byte(`{"v":1,"type":"storage.result","id":"s1","payload":{"id":"s1","ok":true,"value":{"type":"bytes","value":"AAH/","size":3}}}`))
	if err != nil {
		t.Fatal(err)
	}
	var res protocol.StorageResult
	if err := msg.PayloadInto(&res); err != nil {
		t.Fatal(err)
	}
	if res.Value == nil || res.Value.Type != protocol.StorageValueBytes || res.Value.Value != "AAH/" || res.Value.Size != 3 {
		t.Fatalf("value = %+v", res.Value)
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
