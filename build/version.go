// Package build exposes the build configuration that ships inside the binary.
//
// build/config.yml is the single place where the application version lives:
// release-please bumps it on every release and the Wails tooling derives the
// platform assets from it. Embedding it here lets every build, including
// `go run ./cmd/rnlens` and `task dev`, report that version without an
// extra linker flag.
package build

import (
	_ "embed"
	"strings"
)

//go:embed config.yml
var config string

// Version returns the `info.version` value declared in build/config.yml,
// or "dev" when the file carries no version.
func Version() string {
	return parseVersion(config)
}

// parseVersion extracts `info.version` from the YAML in src with a line scan,
// so the binary does not need a YAML dependency for one field.
func parseVersion(src string) string {
	inInfo := false
	for _, line := range strings.Split(src, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
			inInfo = strings.HasPrefix(trimmed, "info:")
			continue
		}
		if !inInfo || !strings.HasPrefix(trimmed, "version:") {
			continue
		}
		value := strings.TrimSpace(strings.TrimPrefix(trimmed, "version:"))
		if i := strings.Index(value, "#"); i >= 0 {
			value = strings.TrimSpace(value[:i])
		}
		if value = strings.Trim(value, `"'`); value != "" {
			return value
		}
	}
	return "dev"
}
