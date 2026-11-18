package mcpserver

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func connectControl(t *testing.T, lens Lens, opts Options) (*mcp.ClientSession, *Control) {
	t.Helper()
	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	server, control := NewServer(lens, opts)
	ss, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ss.Close() })
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "claude-code", Version: "2.1"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs, control
}

func TestEveryToolCallIsRecorded(t *testing.T) {
	t.Parallel()
	notified := 0
	cs, control := connectControl(t, oneDevice(), Options{OnChange: func() { notified++ }})
	mustCall(t, cs, "status", nil)
	mustCall(t, cs, "set_storage_value", map[string]any{"store": "async-storage", "key": "theme", "value": "dark"})
	if _, isErr := call(t, cs, "query_action", map[string]any{"action": "EXPLODE", "queryHash": "x"}); !isErr {
		t.Fatal("bad action succeeded")
	}
	calls := control.Activity(0)
	if len(calls) != 3 {
		t.Fatalf("recorded %d calls", len(calls))
	}
	first, write, failed := calls[0], calls[1], calls[2]
	if first.Tool != "status" || first.Client != "claude-code 2.1" || first.Kind != KindRead || first.Error != "" || first.Seq != 1 {
		t.Fatalf("first = %+v", first)
	}
	if write.Tool != "set_storage_value" || write.Kind != KindDestructive || !strings.Contains(string(write.Args), `"key":"theme"`) || write.Result != "Wrote theme in async-storage." {
		t.Fatalf("write = %+v", write)
	}
	if failed.Error == "" || !strings.Contains(failed.Error, "EXPLODE") {
		t.Fatalf("failed = %+v", failed)
	}
	if got := control.Activity(2); len(got) != 1 || got[0].Seq != 3 {
		t.Fatalf("Activity(2) = %+v", got)
	}
	if notified < 3 {
		t.Fatalf("notified %d times", notified)
	}
	control.ClearActivity()
	if got := control.Activity(0); len(got) != 0 {
		t.Fatalf("after Clear = %+v", got)
	}
	mustCall(t, cs, "status", nil)
	if got := control.Activity(0); len(got) != 1 || got[0].Seq != 4 {
		t.Fatalf("seq after Clear = %+v", got)
	}
}

func TestTheActivityLogIsBounded(t *testing.T) {
	t.Parallel()
	cs, control := connectControl(t, oneDevice(), Options{})
	for range maxActivity + 5 {
		mustCall(t, cs, "list_targets", nil)
	}
	calls := control.Activity(0)
	if len(calls) != maxActivity || calls[0].Seq != 6 {
		t.Fatalf("kept %d calls from seq %d", len(calls), calls[0].Seq)
	}
}

func TestReadOnlyRefusesChanges(t *testing.T) {
	t.Parallel()
	lens := oneDevice()
	cs, control := connectControl(t, lens, Options{ReadOnly: true})
	if !control.ReadOnly() {
		t.Fatal("ReadOnly option not applied")
	}
	mustCall(t, cs, "list_stores", nil)
	for _, tool := range []string{"set_storage_value", "evaluate_js", "attach_target", "set_online"} {
		out, isErr := call(t, cs, tool, map[string]any{"store": "s", "key": "k", "value": "v", "expression": "1", "targetId": "t", "online": true})
		if !isErr || !strings.Contains(out, "read-only") {
			t.Fatalf("%s in read-only mode = %s", tool, out)
		}
	}
	if got := lens.Calls(); len(got) != 0 {
		t.Fatalf("refused calls reached the app: %v", got)
	}
	if last := control.Activity(0); !last[len(last)-1].Refused {
		t.Fatalf("a refused call is not marked: %+v", last[len(last)-1])
	}
	control.SetReadOnly(false)
	mustCall(t, cs, "set_storage_value", map[string]any{"store": "async-storage", "key": "k", "value": "v"})
}

func TestToolsAreDescribed(t *testing.T) {
	t.Parallel()
	_, control := connectControl(t, oneDevice(), Options{})
	tools := control.Tools()
	if len(tools) != 21 {
		t.Fatalf("%d tools", len(tools))
	}
	kinds := map[string]ToolKind{}
	for _, tool := range tools {
		kinds[tool.Name] = tool.Kind
		if tool.Description == "" {
			t.Fatalf("%s has no description", tool.Name)
		}
	}
	if kinds["status"] != KindRead || kinds["set_online"] != KindChange || kinds["clear_storage"] != KindDestructive {
		t.Fatalf("kinds = %v", kinds)
	}
	if tools[0].Name != "status" {
		t.Fatalf("tools are not in registration order: %s first", tools[0].Name)
	}
}

// A tool added without addTool is unknown to the gate: in read-only mode it
// is refused rather than trusted.
func TestReadOnlyFailsClosedForUnknownTools(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	server, control := NewServer(oneDevice(), Options{ReadOnly: true})
	ran := false
	mcp.AddTool(server, &mcp.Tool{Name: "sneaky_write"}, func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, any, error) {
		ran = true
		return message("wrote")
	})
	st, ct := mcp.NewInMemoryTransports()
	ss, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ss.Close() })
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "t"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	if out, isErr := call(t, cs, "sneaky_write", nil); !isErr || !strings.Contains(out, "read-only") || ran {
		t.Fatalf("unknown tool in read-only mode = %s (ran %v)", out, ran)
	}
	if last := control.Activity(0); len(last) != 1 || !last[0].Refused {
		t.Fatalf("activity = %+v", last)
	}
}

func TestLargeArgumentsAreCutInTheLog(t *testing.T) {
	t.Parallel()
	cs, control := connectControl(t, oneDevice(), Options{})
	mustCall(t, cs, "evaluate_js", map[string]any{"expression": strings.Repeat("x", 3*summaryLimit)})
	args := control.Activity(0)[0].Args
	if len(args) > summaryLimit+64 {
		t.Fatalf("stored %d bytes of arguments", len(args))
	}
	var s string
	if err := json.Unmarshal(args, &s); err != nil || !strings.HasSuffix(s, "…") {
		t.Fatalf("cut arguments are not a JSON string ending in …: %s (%v)", args, err)
	}
}
