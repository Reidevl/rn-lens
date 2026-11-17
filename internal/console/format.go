package console

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/Reidevl/rn-lens/internal/platform/cdp"
)

// FormatArgs renders console arguments the way Chrome's console does on one
// line: the first string argument may contain format specifiers that consume
// the following arguments; everything else is joined with spaces.
func FormatArgs(args []cdp.RemoteObject) string {
	if len(args) == 0 {
		return ""
	}
	parts := make([]string, 0, len(args))
	rest := args
	if first := args[0]; first.Type == "string" {
		text := stringValue(first)
		if strings.Contains(text, "%") {
			formatted, consumed := applyFormat(text, args[1:])
			parts = append(parts, formatted)
			rest = args[1+consumed:]
		} else {
			parts = append(parts, text)
			rest = args[1:]
		}
	}
	for _, a := range rest {
		parts = append(parts, FormatRemoteObject(a))
	}
	return strings.Join(parts, " ")
}

// applyFormat substitutes %s %d %i %f %o %O %j %c into format, consuming
// arguments in order. It returns the text and how many arguments were used.
func applyFormat(format string, args []cdp.RemoteObject) (string, int) {
	var b strings.Builder
	used := 0
	for i := 0; i < len(format); i++ {
		c := format[i]
		if c != '%' || i+1 >= len(format) {
			b.WriteByte(c)
			continue
		}
		verb := format[i+1]
		switch verb {
		case '%':
			b.WriteByte('%')
			i++
		case 's', 'd', 'i', 'f', 'o', 'O', 'j':
			if used >= len(args) {
				b.WriteByte(c)
				continue
			}
			arg := args[used]
			used++
			i++
			switch verb {
			case 'd', 'i':
				b.WriteString(numberValue(arg, true))
			case 'f':
				b.WriteString(numberValue(arg, false))
			default:
				b.WriteString(FormatRemoteObject(arg))
			}
		case 'c':
			// CSS styling: consume the argument, render nothing.
			if used < len(args) {
				used++
			}
			i++
		default:
			b.WriteByte(c)
		}
	}
	return b.String(), used
}

// FormatRemoteObject renders a single remote object as Chrome would in a
// collapsed console line.
func FormatRemoteObject(o cdp.RemoteObject) string {
	switch o.Type {
	case "string":
		return stringValue(o)
	case "number":
		return numberValue(o, false)
	case "boolean":
		return strings.TrimSpace(string(o.Value))
	case "undefined":
		return "undefined"
	case "bigint":
		if o.UnserializableValue != "" {
			return o.UnserializableValue
		}
		return o.Description
	case "symbol":
		return o.Description
	case "function":
		return firstLine(o.Description)
	case "object":
		return formatObject(o)
	default:
		if o.Description != "" {
			return o.Description
		}
		return o.Type
	}
}

func formatObject(o cdp.RemoteObject) string {
	if o.Subtype == "null" {
		return "null"
	}
	if o.Preview != nil {
		return formatPreview(o.Preview)
	}
	switch o.Subtype {
	case "error":
		return firstLine(o.Description)
	case "array":
		if o.Description != "" {
			return o.Description // "Array(3)"
		}
		return "Array"
	}
	if o.Description != "" {
		return o.Description
	}
	if o.ClassName != "" {
		return o.ClassName
	}
	return "Object"
}

func formatPreview(p *cdp.ObjectPreview) string {
	parts := make([]string, 0, len(p.Properties)+1)
	isArray := p.Subtype == "array"
	for _, prop := range p.Properties {
		val := previewValue(prop)
		if isArray {
			parts = append(parts, val)
		} else {
			parts = append(parts, prop.Name+": "+val)
		}
	}
	if p.Overflow {
		parts = append(parts, "…")
	}
	if isArray {
		return "(" + strconv.Itoa(len(p.Properties)) + ") [" + strings.Join(parts, ", ") + "]"
	}
	prefix := ""
	if p.Description != "" && p.Description != "Object" {
		prefix = p.Description + " "
	}
	return prefix + "{" + strings.Join(parts, ", ") + "}"
}

func previewValue(prop cdp.PropertyPreview) string {
	switch prop.Type {
	case "string":
		return "'" + prop.Value + "'"
	case "object":
		if prop.ValuePreview != nil {
			return formatPreview(prop.ValuePreview)
		}
		if prop.Subtype == "null" {
			return "null"
		}
		if prop.Subtype == "array" {
			return "Array"
		}
		return "{…}"
	case "function":
		return "ƒ"
	default:
		if prop.Value == "" {
			return prop.Type
		}
		return prop.Value
	}
}

func stringValue(o cdp.RemoteObject) string {
	var s string
	if err := json.Unmarshal(o.Value, &s); err == nil {
		return s
	}
	return o.Description
}

func numberValue(o cdp.RemoteObject, integer bool) string {
	if o.UnserializableValue != "" {
		return o.UnserializableValue
	}
	var f float64
	if err := json.Unmarshal(o.Value, &f); err == nil {
		if integer {
			return strconv.FormatInt(int64(f), 10)
		}
		return strconv.FormatFloat(f, 'f', -1, 64)
	}
	if s := stringValue(o); s != "" {
		return s
	}
	return "NaN"
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
