package app

import (
	"testing"

	"github.com/Reidevl/rn-lens/internal/adapters/metro"
	"github.com/Reidevl/rn-lens/internal/targets"
)

func TestTargetFromMetro(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   metro.Discovered
		want targets.Target
	}{
		{
			name: "fusebox page carries identity and capabilities",
			in: metro.Discovered{
				Endpoint: "http://localhost:8081",
				Page: metro.Page{
					ID:                   "device-42-7",
					Title:                "React Native",
					AppID:                "com.example.app",
					DeviceName:           "iPhone 17",
					WebSocketDebuggerURL: "ws://localhost:8081/inspector/debug?device=device-42&page=7",
					VM:                   "Hermes",
					ReactNative: metro.ReactNativeInfo{
						LogicalDeviceID: "device-42",
						Capabilities: metro.Capabilities{
							NativePageReloads:         true,
							NativeSourceCodeFetching:  true,
							SupportsMultipleDebuggers: true,
						},
					},
				},
			},
			want: targets.Target{
				ID:              "http://localhost:8081|device-42-7",
				Endpoint:        "http://localhost:8081",
				MetroID:         "device-42-7",
				PageID:          "7",
				Title:           "React Native",
				AppID:           "com.example.app",
				DeviceName:      "iPhone 17",
				LogicalDeviceID: "device-42",
				WebSocketURL:    "ws://localhost:8081/inspector/debug?device=device-42&page=7",
				VM:              "Hermes",
				Capabilities: targets.Capabilities{
					NativePageReloads:         true,
					NativeSourceCodeFetching:  true,
					SupportsMultipleDebuggers: true,
				},
				Reloadable: true,
			},
		},
		{
			name: "legacy page is reloadable by title only",
			in: metro.Discovered{
				Endpoint: "http://localhost:8082",
				Page:     metro.Page{ID: "x", Title: "React Native Experimental (Improved Chrome Reloads)"},
			},
			want: targets.Target{
				ID:         "http://localhost:8082|x",
				Endpoint:   "http://localhost:8082",
				MetroID:    "x",
				PageID:     "x",
				Title:      "React Native Experimental (Improved Chrome Reloads)",
				Reloadable: true,
			},
		},
		{
			name: "plain page is not reloadable",
			in: metro.Discovered{
				Endpoint: "http://localhost:8082",
				Page:     metro.Page{ID: "y", Title: "Hermes React Native"},
			},
			want: targets.Target{
				ID:       "http://localhost:8082|y",
				Endpoint: "http://localhost:8082",
				MetroID:  "y",
				PageID:   "y",
				Title:    "Hermes React Native",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := targetFromMetro(tt.in); got != tt.want {
				t.Errorf("targetFromMetro() = %+v, want %+v", got, tt.want)
			}
		})
	}
}
