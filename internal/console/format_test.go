package console_test

import (
	"encoding/json"
	"testing"

	"github.com/Reidevl/rn-lens/internal/console"
	"github.com/Reidevl/rn-lens/internal/platform/cdp"
)

func str(s string) cdp.RemoteObject {
	v, _ := json.Marshal(s)
	return cdp.RemoteObject{Type: "string", Value: v}
}

func num(n float64) cdp.RemoteObject {
	v, _ := json.Marshal(n)
	return cdp.RemoteObject{Type: "number", Value: v}
}

func TestFormatRemoteObject(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   cdp.RemoteObject
		want string
	}{
		{"string is unquoted", str("hello"), "hello"},
		{"integer", num(42), "42"},
		{"float keeps precision", num(1.5), "1.5"},
		{"NaN", cdp.RemoteObject{Type: "number", UnserializableValue: "NaN"}, "NaN"},
		{"boolean", cdp.RemoteObject{Type: "boolean", Value: json.RawMessage("true")}, "true"},
		{"undefined", cdp.RemoteObject{Type: "undefined"}, "undefined"},
		{"null", cdp.RemoteObject{Type: "object", Subtype: "null", Value: json.RawMessage("null")}, "null"},
		{"bigint", cdp.RemoteObject{Type: "bigint", UnserializableValue: "10n"}, "10n"},
		{"function uses first line", cdp.RemoteObject{Type: "function", Description: "function add(a, b) {\n  return a + b;\n}"}, "function add(a, b) {"},
		{"error uses first line of description", cdp.RemoteObject{Type: "object", Subtype: "error", Description: "Error: boom\n    at x (file:1:2)"}, "Error: boom"},
		{"array without preview uses description", cdp.RemoteObject{Type: "object", Subtype: "array", Description: "Array(3)"}, "Array(3)"},
		{"object without preview uses description", cdp.RemoteObject{Type: "object", ClassName: "Object", Description: "Object", ObjectID: "1"}, "Object"},
		{"object with preview", cdp.RemoteObject{Type: "object", Preview: &cdp.ObjectPreview{Type: "object", Description: "Object", Properties: []cdp.PropertyPreview{{Name: "n", Type: "number", Value: "1"}, {Name: "s", Type: "string", Value: "x"}, {Name: "o", Type: "object", Subtype: "null"}}}}, "{n: 1, s: 'x', o: null}"},
		{"array with preview and overflow", cdp.RemoteObject{Type: "object", Subtype: "array", Preview: &cdp.ObjectPreview{Type: "object", Subtype: "array", Overflow: true, Properties: []cdp.PropertyPreview{{Name: "0", Type: "number", Value: "1"}, {Name: "1", Type: "number", Value: "2"}}}}, "(2) [1, 2, …]"},
		{"class instance keeps its name", cdp.RemoteObject{Type: "object", Preview: &cdp.ObjectPreview{Type: "object", Description: "Point", Properties: []cdp.PropertyPreview{{Name: "x", Type: "number", Value: "0"}}}}, "Point {x: 0}"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := console.FormatRemoteObject(tt.in); got != tt.want {
				t.Fatalf("FormatRemoteObject = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFormatArgs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   []cdp.RemoteObject
		want string
	}{
		{"empty", nil, ""},
		{"joins with spaces", []cdp.RemoteObject{str("tick"), num(3), {Type: "object", Description: "Object"}}, "tick 3 Object"},
		{"%s and %d substitute", []cdp.RemoteObject{str("user %s has %d items"), str("ana"), num(3)}, "user ana has 3 items"},
		{"%i truncates", []cdp.RemoteObject{str("n=%i"), num(2.9)}, "n=2"},
		{"%c consumes the style argument", []cdp.RemoteObject{str("%cstyled"), str("color: red")}, "styled"},
		{"%% is a literal", []cdp.RemoteObject{str("100%% done")}, "100% done"},
		{"missing argument keeps the specifier", []cdp.RemoteObject{str("a %s b")}, "a %s b"},
		{"extra arguments are appended", []cdp.RemoteObject{str("x=%d"), num(1), str("tail")}, "x=1 tail"},
		{"first non-string argument is not a format", []cdp.RemoteObject{num(1), str("%s")}, "1 %s"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := console.FormatArgs(tt.in); got != tt.want {
				t.Fatalf("FormatArgs = %q, want %q", got, tt.want)
			}
		})
	}
}
