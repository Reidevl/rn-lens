package cdp

import "encoding/json"

// This file holds the subset of the Runtime and Log domains Lens decodes.
// It is written by hand on purpose: Hermes implements a slice of CDP, the
// generated chromedp/cdproto packages change with every Chromium release, and
// the recorded fixture in internal/adapters/cdpws/testdata pins these shapes.

// Runtime and Log method and event names used by the console feature.
const (
	MethodLogEnable                     = "Log.enable"
	MethodRuntimeEvaluate               = "Runtime.evaluate"
	MethodRuntimeGetProperties          = "Runtime.getProperties"
	MethodRuntimeReleaseObject          = "Runtime.releaseObject"
	EventRuntimeConsoleAPICalled        = "Runtime.consoleAPICalled"
	EventRuntimeExceptionThrown         = "Runtime.exceptionThrown"
	EventRuntimeExecutionContextCreated = "Runtime.executionContextCreated"
	EventRuntimeExecutionContextsClear  = "Runtime.executionContextsCleared"
	EventLogEntryAdded                  = "Log.entryAdded"
)

// RemoteObject mirrors Runtime.RemoteObject. Primitive values arrive in Value;
// objects carry an ObjectID that stays valid until the execution context is
// cleared. Hermes does not send Preview unless asked with generatePreview.
type RemoteObject struct {
	Type                string          `json:"type"`
	Subtype             string          `json:"subtype,omitempty"`
	ClassName           string          `json:"className,omitempty"`
	Value               json.RawMessage `json:"value,omitempty"`
	UnserializableValue string          `json:"unserializableValue,omitempty"`
	Description         string          `json:"description,omitempty"`
	ObjectID            string          `json:"objectId,omitempty"`
	Preview             *ObjectPreview  `json:"preview,omitempty"`
}

// ObjectPreview mirrors Runtime.ObjectPreview.
type ObjectPreview struct {
	Type        string            `json:"type"`
	Subtype     string            `json:"subtype,omitempty"`
	Description string            `json:"description,omitempty"`
	Overflow    bool              `json:"overflow"`
	Properties  []PropertyPreview `json:"properties"`
}

// PropertyPreview mirrors Runtime.PropertyPreview.
type PropertyPreview struct {
	Name         string         `json:"name"`
	Type         string         `json:"type"`
	Value        string         `json:"value,omitempty"`
	ValuePreview *ObjectPreview `json:"valuePreview,omitempty"`
	Subtype      string         `json:"subtype,omitempty"`
}

// CallFrame mirrors Runtime.CallFrame.
type CallFrame struct {
	FunctionName string `json:"functionName"`
	ScriptID     string `json:"scriptId"`
	URL          string `json:"url"`
	LineNumber   int    `json:"lineNumber"`
	ColumnNumber int    `json:"columnNumber"`
}

// StackTrace mirrors Runtime.StackTrace.
type StackTrace struct {
	Description string      `json:"description,omitempty"`
	CallFrames  []CallFrame `json:"callFrames"`
	Parent      *StackTrace `json:"parent,omitempty"`
}

// ExceptionDetails mirrors Runtime.ExceptionDetails.
type ExceptionDetails struct {
	ExceptionID        int           `json:"exceptionId"`
	Text               string        `json:"text"`
	LineNumber         int           `json:"lineNumber"`
	ColumnNumber       int           `json:"columnNumber"`
	ScriptID           string        `json:"scriptId,omitempty"`
	URL                string        `json:"url,omitempty"`
	StackTrace         *StackTrace   `json:"stackTrace,omitempty"`
	Exception          *RemoteObject `json:"exception,omitempty"`
	ExecutionContextID int           `json:"executionContextId,omitempty"`
}

// ConsoleAPICalled mirrors the params of Runtime.consoleAPICalled.
type ConsoleAPICalled struct {
	Type               string         `json:"type"`
	Args               []RemoteObject `json:"args"`
	ExecutionContextID int            `json:"executionContextId"`
	Timestamp          float64        `json:"timestamp"` // ms since epoch; Hermes may omit it
	StackTrace         *StackTrace    `json:"stackTrace,omitempty"`
	Context            string         `json:"context,omitempty"`
}

// ExceptionThrown mirrors the params of Runtime.exceptionThrown.
type ExceptionThrown struct {
	Timestamp        float64          `json:"timestamp"`
	ExceptionDetails ExceptionDetails `json:"exceptionDetails"`
}

// ExecutionContextDescription mirrors Runtime.ExecutionContextDescription.
type ExecutionContextDescription struct {
	ID     int    `json:"id"`
	Origin string `json:"origin"`
	Name   string `json:"name"`
}

// ExecutionContextCreated mirrors the params of Runtime.executionContextCreated.
type ExecutionContextCreated struct {
	Context ExecutionContextDescription `json:"context"`
}

// LogEntry mirrors Log.LogEntry.
type LogEntry struct {
	Source     string      `json:"source"`
	Level      string      `json:"level"`
	Text       string      `json:"text"`
	Timestamp  float64     `json:"timestamp"`
	URL        string      `json:"url,omitempty"`
	LineNumber int         `json:"lineNumber,omitempty"`
	StackTrace *StackTrace `json:"stackTrace,omitempty"`
}

// LogEntryAdded mirrors the params of Log.entryAdded.
type LogEntryAdded struct {
	Entry LogEntry `json:"entry"`
}

// EvaluateParams are the params Lens sends with Runtime.evaluate.
type EvaluateParams struct {
	Expression            string `json:"expression"`
	IncludeCommandLineAPI bool   `json:"includeCommandLineAPI,omitempty"`
	GeneratePreview       bool   `json:"generatePreview,omitempty"`
	AwaitPromise          bool   `json:"awaitPromise,omitempty"`
	ReturnByValue         bool   `json:"returnByValue,omitempty"`
	ReplMode              bool   `json:"replMode,omitempty"`
}

// EvaluateResult is the result of Runtime.evaluate.
type EvaluateResult struct {
	Result           RemoteObject      `json:"result"`
	ExceptionDetails *ExceptionDetails `json:"exceptionDetails,omitempty"`
}

// GetPropertiesParams are the params Lens sends with Runtime.getProperties.
type GetPropertiesParams struct {
	ObjectID               string `json:"objectId"`
	OwnProperties          bool   `json:"ownProperties,omitempty"`
	AccessorPropertiesOnly bool   `json:"accessorPropertiesOnly,omitempty"`
	GeneratePreview        bool   `json:"generatePreview,omitempty"`
}

// PropertyDescriptor mirrors Runtime.PropertyDescriptor.
type PropertyDescriptor struct {
	Name         string        `json:"name"`
	Value        *RemoteObject `json:"value,omitempty"`
	Writable     bool          `json:"writable"`
	Get          *RemoteObject `json:"get,omitempty"`
	Set          *RemoteObject `json:"set,omitempty"`
	Configurable bool          `json:"configurable"`
	Enumerable   bool          `json:"enumerable"`
	IsOwn        bool          `json:"isOwn"`
	Symbol       *RemoteObject `json:"symbol,omitempty"`
}

// GetPropertiesResult is the result of Runtime.getProperties.
type GetPropertiesResult struct {
	Result             []PropertyDescriptor `json:"result"`
	InternalProperties []PropertyDescriptor `json:"internalProperties,omitempty"`
	ExceptionDetails   *ExceptionDetails    `json:"exceptionDetails,omitempty"`
}
