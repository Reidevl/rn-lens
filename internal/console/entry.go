// Package console owns the console feature: a bounded log of everything the
// attached app prints or throws, the mapping from CDP events to entries, and
// evaluation of expressions in the app.
package console

import (
	"time"

	"github.com/Reidevl/rn-lens/internal/platform/cdp"
)

// Kind is the console method or event type an entry came from.
type Kind string

// Entry kinds. Console API kinds follow Runtime.consoleAPICalled.type.
const (
	KindLog            Kind = "log"
	KindDebug          Kind = "debug"
	KindInfo           Kind = "info"
	KindWarning        Kind = "warning"
	KindError          Kind = "error"
	KindDir            Kind = "dir"
	KindDirXML         Kind = "dirxml"
	KindTable          Kind = "table"
	KindTrace          Kind = "trace"
	KindClear          Kind = "clear"
	KindGroup          Kind = "startGroup"
	KindGroupCollapsed Kind = "startGroupCollapsed"
	KindGroupEnd       Kind = "endGroup"
	KindAssert         Kind = "assert"
	KindCount          Kind = "count"
	KindTimeEnd        Kind = "timeEnd"
	KindException      Kind = "exception" // uncaught error from Runtime.exceptionThrown
	KindCommand        Kind = "command"   // expression the user evaluated
	KindResult         Kind = "result"    // value an evaluation produced
	KindSystem         Kind = "system"    // Lens itself (context reset, attach)
)

// Source tells where an entry came from.
type Source string

// Entry sources.
const (
	SourceConsole   Source = "console"   // Runtime.consoleAPICalled
	SourceException Source = "exception" // Runtime.exceptionThrown
	SourceLog       Source = "log"       // Log.entryAdded (network, security, ...)
	SourceEval      Source = "eval"      // Lens evaluation
	SourceSystem    Source = "system"
)

// Location points at the script position an entry refers to.
type Location struct {
	URL    string `json:"url"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
}

// Entry is one console line. Args keep the raw remote objects so the UI can
// expand them lazily; Text is the one-line summary shown by default.
type Entry struct {
	Seq       uint64    `json:"seq"`
	Timestamp time.Time `json:"timestamp"`
	Kind      Kind      `json:"kind"`
	Source    Source    `json:"source"`
	// Level folds kinds into the filter buckets the UI offers.
	Level     Level              `json:"level"`
	Text      string             `json:"text"`
	Args      []cdp.RemoteObject `json:"args,omitempty"`
	Stack     *cdp.StackTrace    `json:"stack,omitempty"`
	Location  *Location          `json:"location,omitempty"`
	ContextID int                `json:"contextId,omitempty"`
	// Epoch identifies the JavaScript context generation the entry belongs
	// to. Object handles from older epochs are stale after a reload.
	Epoch uint64 `json:"epoch"`
	// Exception carries the details for KindException entries.
	Exception *cdp.ExceptionDetails `json:"exception,omitempty"`
}

// Level is the filter bucket of an entry.
type Level string

// Levels, from least to most severe, plus the Lens-owned ones.
const (
	LevelVerbose Level = "verbose"
	LevelInfo    Level = "info"
	LevelWarning Level = "warning"
	LevelError   Level = "error"
	LevelSystem  Level = "system"
)

// LevelOf maps a kind to its filter bucket.
func LevelOf(kind Kind) Level {
	switch kind {
	case KindDebug, KindTrace, KindCount, KindTimeEnd:
		return LevelVerbose
	case KindWarning:
		return LevelWarning
	case KindError, KindException, KindAssert:
		return LevelError
	case KindSystem, KindCommand, KindResult:
		return LevelSystem
	case KindLog, KindInfo, KindDir, KindDirXML, KindTable, KindClear, KindGroup, KindGroupCollapsed, KindGroupEnd:
		return LevelInfo
	default:
		return LevelInfo
	}
}

// kindFromConsoleType normalises Runtime.consoleAPICalled.type.
func kindFromConsoleType(t string) Kind {
	switch t {
	case "warn", "warning":
		return KindWarning
	case "":
		return KindLog
	default:
		return Kind(t)
	}
}

// kindFromLogLevel maps Log.LogEntry.level.
func kindFromLogLevel(level string) Kind {
	switch level {
	case "verbose":
		return KindDebug
	case "warning":
		return KindWarning
	case "error":
		return KindError
	default:
		return KindInfo
	}
}
