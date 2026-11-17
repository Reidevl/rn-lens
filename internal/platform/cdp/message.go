// Package cdp holds the Chrome DevTools Protocol vocabulary shared by every
// feature: events, errors, close reasons and well-known method names.
//
// React Native (Hermes) implements a subset of CDP plus custom domains such as
// ReactNativeApplication, so everything stays generic: methods are strings and
// payloads are raw JSON. Feature packages decode only what they need. The
// WebSocket transport lives in internal/adapters/cdpws.
package cdp

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Message is the wire representation of every CDP frame: a request
// (id+method), a response (id+result|error) or an event (method, no id).
type Message struct {
	ID     *int64          `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *Error          `json:"error,omitempty"`
}

// Event is a notification pushed by the target (no request id).
type Event struct {
	Method string
	Params json.RawMessage
}

// Direction tells a frame observer which way a frame travelled.
type Direction string

// Frame directions as seen from Lens.
const (
	DirectionSend    Direction = "send"
	DirectionReceive Direction = "recv"
)

// Well-known methods and events used across features.
const (
	MethodReactNativeApplicationEnable = "ReactNativeApplication.enable"
	EventReactNativeMetadataUpdated    = "ReactNativeApplication.metadataUpdated"
	MethodRuntimeEnable                = "Runtime.enable"
	MethodPageReload                   = "Page.reload"
)

// Error is a protocol-level error returned for a request.
type Error struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *Error) Error() string {
	return fmt.Sprintf("cdp: %s (code %d)", e.Message, e.Code)
}

// CloseReason explains why a connection ended. Metro's inspector proxy closes
// debugger sockets with reasons such as "[NEW_DEBUGGER_OPENED] New debugger
// opened for the same app instance"; Code holds the bracketed token.
type CloseReason struct {
	Status  int
	Code    string
	Message string
}

// Known close codes emitted by @react-native/dev-middleware.
const (
	CloseNewDebuggerOpened = "NEW_DEBUGGER_OPENED"
	CloseConnectionLost    = "CONNECTION_LOST"
	ClosePageRemoved       = "PAGE_REMOVED"
	CloseRecreatingDevice  = "RECREATING_DEVICE"
	CloseIncorrectURL      = "INCORRECT_URL"
)

func (r CloseReason) String() string {
	switch {
	case r.Code != "" && r.Message != "":
		return fmt.Sprintf("[%s] %s (status %d)", r.Code, r.Message, r.Status)
	case r.Code != "":
		return fmt.Sprintf("[%s] (status %d)", r.Code, r.Status)
	case r.Message != "":
		return fmt.Sprintf("%s (status %d)", r.Message, r.Status)
	default:
		return fmt.Sprintf("closed (status %d)", r.Status)
	}
}

// ParseCloseReason splits a proxy close reason into its bracketed code and
// human message.
func ParseCloseReason(status int, reason string) CloseReason {
	out := CloseReason{Status: status, Message: strings.TrimSpace(reason)}
	if strings.HasPrefix(out.Message, "[") {
		if end := strings.Index(out.Message, "]"); end > 1 {
			out.Code = out.Message[1:end]
			out.Message = strings.TrimSpace(out.Message[end+1:])
		}
	}
	return out
}
