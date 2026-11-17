package build

import (
	"os"
	"regexp"
	"testing"
)

func TestParseVersion(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "reads info.version and ignores the taskfile version",
			src:  "version: '3'\n\ninfo:\n  productName: \"Lens\"\n  version: \"1.2.3\" # x-release-please-version\n",
			want: "1.2.3",
		},
		{
			name: "ignores commented and foreign version keys",
			src:  "version: '3'\ninfo:\n  name: x\n# ios:\n#   version: \"9.9.9\"\nother:\n  version: \"4.5.6\"\n",
			want: "dev",
		},
		{
			name: "falls back to dev when the field is empty",
			src:  "info:\n  version: \"\"\n",
			want: "dev",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := parseVersion(tc.src); got != tc.want {
				t.Fatalf("parseVersion() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestVersionMatchesConfigFile(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile("config.yml")
	if err != nil {
		t.Fatalf("read config.yml: %v", err)
	}
	got := Version()
	if !regexp.MustCompile(`^\d+\.\d+\.\d+`).MatchString(got) {
		t.Fatalf("Version() = %q, want a semantic version", got)
	}
	if got != parseVersion(string(raw)) {
		t.Fatalf("embedded config (%q) differs from build/config.yml on disk (%q)", got, parseVersion(string(raw)))
	}
}
