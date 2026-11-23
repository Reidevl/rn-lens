package console

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/Reidevl/rn-lens/internal/platform/cdp"
)

// ansiEscape matches terminal escape sequences: CSI sequences such as the
// colors React Native puts in its DevTools banner ("\x1b[30m"), OSC
// sequences ending in BEL or ST, and two-byte escapes.
var ansiEscape = regexp.MustCompile(`\x1b(?:\[[\x30-\x3f]*[\x20-\x2f]*[\x40-\x7e]|\][^\x07\x1b]*(?:\x07|\x1b\\)|[\x40-\x5f])`)

// stripANSI removes terminal escape sequences, which the console cannot
// render and would otherwise show as raw text.
func stripANSI(s string) string {
	if !strings.Contains(s, "\x1b") {
		return s
	}
	return ansiEscape.ReplaceAllString(s, "")
}

// stripANSIArgs returns args with escape sequences removed from string
// values. Other arguments are returned unchanged.
func stripANSIArgs(args []cdp.RemoteObject) []cdp.RemoteObject {
	out := args
	copied := false
	for i, a := range args {
		if a.Type != "string" {
			continue
		}
		var s string
		if err := json.Unmarshal(a.Value, &s); err != nil || !strings.Contains(s, "\x1b") {
			continue
		}
		raw, err := json.Marshal(stripANSI(s))
		if err != nil {
			continue
		}
		if !copied {
			out = append([]cdp.RemoteObject(nil), args...)
			copied = true
		}
		out[i].Value = raw
	}
	return out
}
