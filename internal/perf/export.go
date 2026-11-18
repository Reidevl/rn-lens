package perf

import (
	"bufio"
	"io"
)

// WriteTrace writes t as a Chrome JSON trace, {"traceEvents": [...]}, the
// format Perfetto UI and Chrome DevTools open.
func WriteTrace(w io.Writer, t Trace) error {
	bw := bufio.NewWriter(w)
	_, _ = bw.WriteString(`{"traceEvents":[`)
	for i, ev := range t.Events {
		if i > 0 {
			_ = bw.WriteByte(',')
		}
		_, _ = bw.Write(ev)
	}
	_, _ = bw.WriteString("]}\n")
	return bw.Flush()
}
