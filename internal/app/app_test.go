package app_test

import (
	"testing"

	"github.com/Reidevl/rn-lens/internal/app"
)

func TestInfo(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		cfg         app.Config
		wantVersion string
	}{
		{name: "reports the configured version", cfg: app.Config{Version: "1.2.3"}, wantVersion: "1.2.3"},
		{name: "defaults an empty version to dev", cfg: app.Config{}, wantVersion: "dev"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := app.New(tt.cfg).Info()

			if got.Name != "Lens" {
				t.Errorf("Info().Name = %q, want %q", got.Name, "Lens")
			}
			if got.Version != tt.wantVersion {
				t.Errorf("Info().Version = %q, want %q", got.Version, tt.wantVersion)
			}
		})
	}
}
