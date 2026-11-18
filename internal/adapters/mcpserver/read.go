package mcpserver

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Reidevl/rn-lens/internal/console"
	"github.com/Reidevl/rn-lens/internal/network"
	"github.com/Reidevl/rn-lens/internal/protocol"
	"github.com/Reidevl/rn-lens/internal/queries"
	"github.com/Reidevl/rn-lens/internal/targets"
)

// bodyLimit bounds one body in network_request.
const bodyLimit = 64 << 10

type deviceArg struct {
	Device string `json:"device,omitempty" jsonschema:"device key or app id from status; optional when one device is connected"`
}

type consoleArgs struct {
	Level  string `json:"level,omitempty" jsonschema:"only entries of this level: verbose (console.debug), info (console.log and console.info), warning, error or system (Lens's own notes and evaluate_js commands and results); debug, log and warn are accepted too"`
	Search string `json:"search,omitempty" jsonschema:"only entries whose text contains this (case-insensitive)"`
	Limit  int    `json:"limit,omitempty" jsonschema:"how many of the newest matching entries to return (default 100, at most 1000)"`
}

type networkListArgs struct {
	Search string `json:"search,omitempty" jsonschema:"only requests whose URL contains this (case-insensitive)"`
	Limit  int    `json:"limit,omitempty" jsonschema:"how many of the newest requests to return (default 50, at most 500)"`
}

type seqArg struct {
	Seq uint64 `json:"seq" jsonschema:"the request's seq from network_requests"`
}

type queryListArgs struct {
	deviceArg
	Search string `json:"search,omitempty" jsonschema:"only queries whose key contains this (case-insensitive)"`
}

type queryArgs struct {
	deviceArg
	QueryHash string `json:"queryHash" jsonschema:"the query's queryHash from list_queries"`
}

type storageGetArgs struct {
	deviceArg
	Store string `json:"store" jsonschema:"store id from list_stores, for example async-storage or mmkv:default"`
	Key   string `json:"key" jsonschema:"a key list_stores shows for that store"`
}

type screenshotArgs struct {
	Format string `json:"format,omitempty" jsonschema:"png (default) or jpeg"`
}

func clamp(n, def, most int) int {
	if n <= 0 {
		return def
	}
	return min(n, most)
}

func addReadTools(s *mcp.Server, lens Lens) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "status", Annotations: readOnly,
		Description: "Lens version, the attached app (Metro target, session state, React Native metadata) and the devices running @rnlens/sdk with their features (queries, storage, plugins).",
	},
		func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, any, error) {
			snap := lens.Session()
			return result(map[string]any{
				"lens":       lens.Info(),
				"session":    snap,
				"sdkDevices": lens.SDKDevices(),
			})
		})

	mcp.AddTool(s, &mcp.Tool{
		Name: "list_targets", Annotations: readOnly,
		Description: "Debuggable React Native apps Metro lists; attach_target attaches Lens to one.",
	},
		func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, any, error) {
			list := lens.Targets()
			if list == nil {
				list = []targets.Target{}
			}
			return result(list)
		})

	mcp.AddTool(s, &mcp.Tool{
		Name: "console_logs", Annotations: readOnly,
		Description: "The attached app's console: logs, warnings, errors and exceptions, newest last.",
	},
		func(_ context.Context, _ *mcp.CallToolRequest, in consoleArgs) (*mcp.CallToolResult, any, error) {
			limit := clamp(in.Limit, 100, 1000)
			level, ok := levelAliases[strings.ToLower(in.Level)]
			if !ok {
				return nil, nil, fmt.Errorf("unknown level %q: use verbose, info, warning, error or system", in.Level)
			}
			search := strings.ToLower(in.Search)
			type line struct {
				Seq   uint64    `json:"seq"`
				Time  time.Time `json:"time"`
				Kind  string    `json:"kind"`
				Level string    `json:"level,omitempty"`
				Text  string    `json:"text"`
				Stack string    `json:"stack,omitempty"`
			}
			out := []line{}
			for _, e := range lens.ConsoleTail(10000) {
				if level != "" && e.Level != level {
					continue
				}
				if search != "" && !strings.Contains(strings.ToLower(e.Text), search) {
					continue
				}
				out = append(out, line{Seq: e.Seq, Time: e.Timestamp, Kind: string(e.Kind), Level: string(e.Level), Text: e.Text, Stack: stackOf(e)})
			}
			if len(out) > limit {
				out = out[len(out)-limit:]
			}
			return result(out)
		})

	mcp.AddTool(s, &mcp.Tool{
		Name: "network_requests", Annotations: readOnly,
		Description: "HTTP requests the attached app made (React Native 0.83+), newest first: method, URL, status, type, size and time.",
	},
		func(_ context.Context, _ *mcp.CallToolRequest, in networkListArgs) (*mcp.CallToolResult, any, error) {
			limit := clamp(in.Limit, 50, 500)
			search := strings.ToLower(in.Search)
			out := make([]network.Summary, 0, limit)
			for _, r := range lens.NetworkRequests() {
				if search != "" && !strings.Contains(strings.ToLower(r.URL), search) {
					continue
				}
				if out = append(out, r); len(out) == limit {
					break
				}
			}
			return result(out)
		})

	mcp.AddTool(s, &mcp.Tool{
		Name: "network_request", Annotations: readOnly,
		Description: "One request with its headers, timing, and request and response bodies (each cut at 64 KiB).",
	},
		func(ctx context.Context, _ *mcp.CallToolRequest, in seqArg) (*mcp.CallToolResult, any, error) {
			req, ok := lens.NetworkRequest(in.Seq)
			if !ok {
				return nil, nil, fmt.Errorf("no request with seq %d; list them with network_requests", in.Seq)
			}
			out := map[string]any{"request": req}
			if body, err := lens.RequestBody(ctx, req.Seq, req.Epoch); err == nil && body != "" {
				out["requestBody"] = cut(body)
			}
			if body, err := lens.ResponseBody(ctx, req.Seq, req.Epoch); err == nil {
				body.Text = cut(body.Text)
				out["responseBody"] = body
			} else {
				out["responseBodyError"] = err.Error()
			}
			return result(out)
		})

	mcp.AddTool(s, &mcp.Tool{
		Name: "list_queries", Annotations: readOnly,
		Description: "TanStack queries of a device: key, hash, status (fresh, stale, fetching, paused, inactive), observers and update times, without their data (use get_query).",
	},
		func(_ context.Context, _ *mcp.CallToolRequest, in queryListArgs) (*mcp.CallToolResult, any, error) {
			dev, err := pickDevice(in.Device, queryKeys(lens))
			if err != nil {
				return nil, nil, err
			}
			list, err := lens.Queries(dev)
			if err != nil {
				return nil, nil, err
			}
			search := strings.ToLower(in.Search)
			type row struct {
				QueryHash     string        `json:"queryHash"`
				QueryKey      any           `json:"queryKey"`
				Label         queries.Label `json:"label"`
				Status        string        `json:"status"`
				FetchStatus   string        `json:"fetchStatus"`
				Observers     int           `json:"observers"`
				DataUpdatedAt float64       `json:"dataUpdatedAt"`
				Error         any           `json:"error,omitempty"`
			}
			out := make([]row, 0, len(list))
			for _, q := range list {
				if search != "" && !strings.Contains(strings.ToLower(string(q.QueryKey)), search) {
					continue
				}
				out = append(out, row{
					QueryHash: q.QueryHash, QueryKey: q.QueryKey, Label: q.Label,
					Status: q.State.Status, FetchStatus: q.State.FetchStatus, Observers: q.ObserversCount,
					DataUpdatedAt: q.State.DataUpdatedAt,
				})
				if len(q.State.Error) > 0 {
					out[len(out)-1].Error = q.State.Error
				}
			}
			return result(map[string]any{"device": dev, "queries": out})
		})

	mcp.AddTool(s, &mcp.Tool{
		Name: "get_query", Annotations: readOnly,
		Description: "One query with its full state and data.",
	},
		func(_ context.Context, _ *mcp.CallToolRequest, in queryArgs) (*mcp.CallToolResult, any, error) {
			dev, err := pickDevice(in.Device, queryKeys(lens))
			if err != nil {
				return nil, nil, err
			}
			q, err := lens.Query(dev, in.QueryHash)
			if err != nil {
				return nil, nil, err
			}
			return result(q)
		})

	mcp.AddTool(s, &mcp.Tool{
		Name: "list_mutations", Annotations: readOnly,
		Description: "TanStack mutations of a device with their state, variables and result.",
	},
		func(_ context.Context, _ *mcp.CallToolRequest, in deviceArg) (*mcp.CallToolResult, any, error) {
			dev, err := pickDevice(in.Device, queryKeys(lens))
			if err != nil {
				return nil, nil, err
			}
			list, err := lens.Mutations(dev)
			if err != nil {
				return nil, nil, err
			}
			return result(map[string]any{"device": dev, "mutations": list})
		})

	mcp.AddTool(s, &mcp.Tool{
		Name: "list_stores", Annotations: readOnly,
		Description: "Storage of a device (AsyncStorage, MMKV instances, SecureStore): each store with its keys and sizes. Read a value with get_storage_value.",
	},
		func(_ context.Context, _ *mcp.CallToolRequest, in deviceArg) (*mcp.CallToolResult, any, error) {
			dev, err := pickDevice(in.Device, storageKeys(lens))
			if err != nil {
				return nil, nil, err
			}
			stores, err := lens.Stores(dev)
			if err != nil {
				return nil, nil, err
			}
			return result(map[string]any{"device": dev, "stores": stores})
		})

	mcp.AddTool(s, &mcp.Tool{
		Name: "get_storage_value", Annotations: readOnly,
		Description: "One stored value as {type, value}: strings as is, numbers and booleans in JSON form, bytes as base64.",
	},
		func(ctx context.Context, _ *mcp.CallToolRequest, in storageGetArgs) (*mcp.CallToolResult, any, error) {
			dev, err := pickDevice(in.Device, storageKeys(lens))
			if err != nil {
				return nil, nil, err
			}
			v, err := lens.StorageGet(ctx, dev, in.Store, in.Key)
			if err != nil {
				return nil, nil, err
			}
			return result(v)
		})

	mcp.AddTool(s, &mcp.Tool{
		Name: "list_plugins", Annotations: readOnly,
		Description: "Panels the app added to Lens with createLensPanel, with their current blocks (tables, key/value lists, logs, buttons). plugin_action clicks a button.",
	},
		func(_ context.Context, _ *mcp.CallToolRequest, in deviceArg) (*mcp.CallToolResult, any, error) {
			dev, err := pickDevice(in.Device, pluginKeys(lens))
			if err != nil {
				return nil, nil, err
			}
			list, err := lens.Plugins(dev)
			if err != nil {
				return nil, nil, err
			}
			return result(map[string]any{"device": dev, "plugins": list})
		})

	mcp.AddTool(s, &mcp.Tool{
		Name: "take_screenshot", Annotations: readOnly,
		Description: "A screenshot of what the attached app shows right now.",
	},
		func(ctx context.Context, _ *mcp.CallToolRequest, in screenshotArgs) (*mcp.CallToolResult, any, error) {
			format := in.Format
			if format == "" {
				format = "png"
			}
			if format != "png" && format != "jpeg" {
				return nil, nil, fmt.Errorf("format must be png or jpeg, not %q", format)
			}
			img, err := lens.Screenshot(ctx, format)
			if err != nil {
				return nil, nil, err
			}
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.ImageContent{Data: img.Data, MIMEType: "image/" + img.Format}}}, nil, nil
		})
}

func cut(s string) string {
	if len(s) <= bodyLimit {
		return s
	}
	return truncateUTF8(s, bodyLimit) + "… (cut at 64 KiB)"
}

// levelAliases maps the level names an AI may use to the console store's
// buckets; "" means every level.
var levelAliases = map[string]console.Level{
	"":        "",
	"verbose": console.LevelVerbose, "debug": console.LevelVerbose,
	"info": console.LevelInfo, "log": console.LevelInfo,
	"warning": console.LevelWarning, "warn": console.LevelWarning,
	"error":  console.LevelError,
	"system": console.LevelSystem,
}

func stackOf(e console.Entry) string {
	if e.Stack == nil {
		return ""
	}
	var b strings.Builder
	for i, f := range e.Stack.CallFrames {
		if i == 8 {
			b.WriteString("    …\n")
			break
		}
		fmt.Fprintf(&b, "    at %s (%s:%d:%d)\n", f.FunctionName, f.URL, f.LineNumber+1, f.ColumnNumber+1)
	}
	return b.String()
}

// queryKeys lists the devices that run the queries feature.
func queryKeys(lens Lens) []string {
	var keys []string
	for _, d := range lens.SDKDevices() {
		for _, f := range d.Features {
			if f == protocol.FeatureQueries {
				keys = append(keys, d.Key)
				break
			}
		}
	}
	return keys
}

func storageKeys(lens Lens) []string {
	devs := lens.StorageDevices()
	keys := make([]string, 0, len(devs))
	for _, d := range devs {
		keys = append(keys, d.Key)
	}
	return keys
}

func pluginKeys(lens Lens) []string {
	devs := lens.PluginDevices()
	keys := make([]string, 0, len(devs))
	for _, d := range devs {
		keys = append(keys, d.Key)
	}
	return keys
}
