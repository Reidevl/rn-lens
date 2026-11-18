package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Reidevl/rn-lens/internal/protocol"
	"github.com/Reidevl/rn-lens/internal/storage"
)

type targetArg struct {
	TargetID string `json:"targetId" jsonschema:"the target's id from list_targets"`
}

type evaluateArgs struct {
	Expression string `json:"expression" jsonschema:"JavaScript to run in the app; the result of the last expression is returned. Promises are awaited."`
}

type queryActionArgs struct {
	deviceArg
	QueryHash string `json:"queryHash,omitempty" jsonschema:"the query's queryHash from list_queries; not needed for CLEAR_QUERY_CACHE and CLEAR_MUTATION_CACHE"`
	Action    string `json:"action" jsonschema:"REFETCH, INVALIDATE, RESET, REMOVE, SET_QUERY_DATA, TRIGGER_ERROR, RESTORE_ERROR, TRIGGER_LOADING, RESTORE_LOADING, CLEAR_QUERY_CACHE or CLEAR_MUTATION_CACHE"`
	Data      any    `json:"data,omitempty" jsonschema:"the new data for SET_QUERY_DATA (any JSON value)"`
}

type onlineArgs struct {
	deviceArg
	Online bool `json:"online" jsonschema:"false makes TanStack Query's onlineManager report the device offline"`
}

type storageSetArgs struct {
	deviceArg
	Store string `json:"store" jsonschema:"store id from list_stores"`
	Key   string `json:"key"`
	Type  string `json:"type,omitempty" jsonschema:"string (default), number, boolean or bytes (value in base64)"`
	Value string `json:"value" jsonschema:"the value as text: numbers and booleans in JSON form"`
}

type storageKeyArgs struct {
	deviceArg
	Store string `json:"store" jsonschema:"store id from list_stores"`
	Key   string `json:"key"`
}

type storageStoreArgs struct {
	deviceArg
	Store string `json:"store" jsonschema:"store id from list_stores"`
}

type pluginActionArgs struct {
	deviceArg
	Plugin string `json:"plugin" jsonschema:"plugin id from list_plugins"`
	Block  string `json:"block" jsonschema:"id of the block that holds the button"`
	Action string `json:"action" jsonschema:"id of the button (an action of an actions block, or a table rowAction)"`
	Row    string `json:"row,omitempty" jsonschema:"row id, for a table row button"`
}

func addWriteTools(s *mcp.Server, lens Lens) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "attach_target", Annotations: changes,
		Description: "Attach Lens to one of the apps list_targets shows.",
	},
		func(_ context.Context, _ *mcp.CallToolRequest, in targetArg) (*mcp.CallToolResult, any, error) {
			if err := lens.Attach(in.TargetID); err != nil {
				return nil, nil, err
			}
			return message("Attaching to %s; check status.", in.TargetID)
		})

	mcp.AddTool(s, &mcp.Tool{
		Name: "reload_app", Annotations: changes,
		Description: "Reload the attached app's JavaScript (like pressing r in Metro).",
	},
		func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
			if err := lens.Reload(ctx); err != nil {
				return nil, nil, err
			}
			return message("The app is reloading.")
		})

	mcp.AddTool(s, &mcp.Tool{
		Name: "evaluate_js", Annotations: destructive,
		Description: "Run JavaScript inside the attached app, like the console's input line. It can read and change anything in the app.",
	},
		func(ctx context.Context, _ *mcp.CallToolRequest, in evaluateArgs) (*mcp.CallToolResult, any, error) {
			if in.Expression == "" {
				return nil, nil, fmt.Errorf("expression is empty")
			}
			e, err := lens.Evaluate(ctx, in.Expression)
			if err != nil {
				return nil, nil, err
			}
			if e.Kind == "exception" {
				return nil, nil, fmt.Errorf("the expression threw: %s", e.Text)
			}
			out := map[string]any{"result": e.Text}
			if len(e.Args) == 1 && len(e.Args[0].Value) > 0 {
				out["value"] = e.Args[0].Value // primitives arrive by value
			}
			return result(out)
		})

	mcp.AddTool(s, &mcp.Tool{
		Name: "query_action", Annotations: destructive,
		Description: "Run a TanStack Query devtools action on the device: refetch, invalidate, reset, remove, set data, trigger or restore the error and loading states, or clear a cache.",
	},
		func(ctx context.Context, _ *mcp.CallToolRequest, in queryActionArgs) (*mcp.CallToolResult, any, error) {
			if !protocol.IsActionType(in.Action) {
				return nil, nil, fmt.Errorf("unknown action %q", in.Action)
			}
			dev, err := pickDevice(in.Device, queryKeys(lens))
			if err != nil {
				return nil, nil, err
			}
			var data json.RawMessage
			if in.Data != nil {
				if data, err = json.Marshal(in.Data); err != nil {
					return nil, nil, err
				}
			}
			if in.Action == "SET_QUERY_DATA" && data == nil {
				return nil, nil, fmt.Errorf("SET_QUERY_DATA needs data")
			}
			if err := lens.QueryAction(ctx, dev, in.Action, in.QueryHash, data); err != nil {
				return nil, nil, err
			}
			return message("%s done on %s.", in.Action, dev)
		})

	mcp.AddTool(s, &mcp.Tool{
		Name: "set_online", Annotations: changes,
		Description: "Switch TanStack Query's onlineManager on the device, to test offline behaviour.",
	},
		func(_ context.Context, _ *mcp.CallToolRequest, in onlineArgs) (*mcp.CallToolResult, any, error) {
			dev, err := pickDevice(in.Device, queryKeys(lens))
			if err != nil {
				return nil, nil, err
			}
			if err := lens.SetOnline(dev, in.Online); err != nil {
				return nil, nil, err
			}
			state := "offline"
			if in.Online {
				state = "online"
			}
			return message("%s is now %s.", dev, state)
		})

	mcp.AddTool(s, &mcp.Tool{
		Name: "set_storage_value", Annotations: destructive,
		Description: "Write one key of a store on the device (adds it or replaces its value).",
	},
		func(ctx context.Context, _ *mcp.CallToolRequest, in storageSetArgs) (*mcp.CallToolResult, any, error) {
			dev, err := pickDevice(in.Device, storageKeys(lens))
			if err != nil {
				return nil, nil, err
			}
			typ := in.Type
			if typ == "" {
				typ = protocol.StorageValueString
			}
			v, err := storage.NormalizeValue(protocol.StorageValue{Type: typ, Value: in.Value})
			if err != nil {
				return nil, nil, err
			}
			if err := lens.StorageSet(ctx, dev, in.Store, in.Key, v); err != nil {
				return nil, nil, err
			}
			return message("Wrote %s in %s.", in.Key, in.Store)
		})

	mcp.AddTool(s, &mcp.Tool{
		Name: "remove_storage_key", Annotations: destructive,
		Description: "Delete one key of a store on the device.",
	},
		func(ctx context.Context, _ *mcp.CallToolRequest, in storageKeyArgs) (*mcp.CallToolResult, any, error) {
			dev, err := pickDevice(in.Device, storageKeys(lens))
			if err != nil {
				return nil, nil, err
			}
			if err := lens.StorageRemove(ctx, dev, in.Store, in.Key); err != nil {
				return nil, nil, err
			}
			return message("Removed %s from %s.", in.Key, in.Store)
		})

	mcp.AddTool(s, &mcp.Tool{
		Name: "clear_storage", Annotations: destructive,
		Description: "Delete every key of a store on the device.",
	},
		func(ctx context.Context, _ *mcp.CallToolRequest, in storageStoreArgs) (*mcp.CallToolResult, any, error) {
			dev, err := pickDevice(in.Device, storageKeys(lens))
			if err != nil {
				return nil, nil, err
			}
			if err := lens.StorageClear(ctx, dev, in.Store); err != nil {
				return nil, nil, err
			}
			return message("Cleared %s.", in.Store)
		})

	mcp.AddTool(s, &mcp.Tool{
		Name: "plugin_action", Annotations: destructive,
		Description: "Click a button of a plugin panel (see list_plugins). Returns the message the app answered with.",
	},
		func(ctx context.Context, _ *mcp.CallToolRequest, in pluginActionArgs) (*mcp.CallToolResult, any, error) {
			dev, err := pickDevice(in.Device, pluginKeys(lens))
			if err != nil {
				return nil, nil, err
			}
			msg, err := lens.PluginAction(ctx, dev, in.Plugin, in.Block, in.Action, in.Row)
			if err != nil {
				return nil, nil, err
			}
			if msg == "" {
				msg = "Done."
			}
			return message("%s", msg)
		})
}
