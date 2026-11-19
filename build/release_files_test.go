package build

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Release Please bumps a "generic" extra-file only on lines that carry an
// `x-release-please-version` marker or sit inside an
// `x-release-please-start-version` … `x-release-please-end` block. Wails
// regenerates some of these files (`wails3 update build-assets`) without the
// markers, and then a release silently ships the old version. This test
// fails as soon as a marker is missing or a marked version drifts.
// It understands only the version markers, not the major/minor/patch/date ones.
func TestReleaseFilesCarryTheReleaseVersion(t *testing.T) {
	var config struct {
		Packages map[string]struct {
			ExtraFiles []json.RawMessage `json:"extra-files"`
		} `json:"packages"`
	}
	readJSON(t, "../release-please-config.json", &config)
	var manifest map[string]string
	readJSON(t, "../.release-please-manifest.json", &manifest)
	want := manifest["."]
	if want == "" {
		t.Fatal(`.release-please-manifest.json has no "." version`)
	}

	semver := regexp.MustCompile(`\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?`)
	generic := 0
	for _, raw := range config.Packages["."].ExtraFiles {
		var file struct{ Type, Path string }
		if err := json.Unmarshal(raw, &file); err != nil || file.Type != "generic" {
			continue // a plain string path or a json/xml updater
		}
		generic++
		t.Run(file.Path, func(t *testing.T) {
			src, err := os.ReadFile(filepath.Join("..", file.Path))
			if err != nil {
				t.Fatal(err)
			}
			marked := 0
			inBlock := false
			for i, line := range strings.Split(string(src), "\n") {
				switch {
				case strings.Contains(line, "x-release-please-start-version"):
					inBlock = true
					continue
				case strings.Contains(line, "x-release-please-end"):
					inBlock = false
					continue
				case !inBlock && !strings.Contains(line, "x-release-please-version"):
					continue
				}
				// Like release-please, only the first version on a line counts.
				if got := semver.FindString(line); got != "" {
					marked++
					if got != want {
						t.Errorf("line %d has version %s, want %s (the manifest version)", i+1, got, want)
					}
				}
			}
			if marked == 0 {
				t.Errorf("no version behind an x-release-please marker; Release Please would never bump this file")
			}
		})
	}
	if generic == 0 {
		t.Fatal("release-please-config.json lists no generic extra-files")
	}
}

func readJSON(t *testing.T, path string, v any) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, v); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
}
