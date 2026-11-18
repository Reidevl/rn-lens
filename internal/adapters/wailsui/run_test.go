package wailsui

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLensRoutesComeBeforeTheAssets(t *testing.T) {
	t.Parallel()
	tag := func(name string) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(name)) })
	}
	h := withLensRoutes(tag("lens"), tag("assets"))
	get := func(path string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, http.NoBody))
		return rec
	}

	want := `{"mode":"desktop"}`
	if browserMode {
		want = `{"mode":"browser"}`
	}
	if rec := get("/lens/mode"); rec.Body.String() != want || rec.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("/lens/mode = %q (%s), want %s", rec.Body.String(), rec.Header().Get("Content-Type"), want)
	}
	for path, handler := range map[string]string{
		"/lens/downloads/t/x.json": "lens",
		"/lens/uploads":            "lens",
		"/":                        "assets",
		"/assets/index.js":         "assets",
		"/lensfoo":                 "assets",
	} {
		if got := get(path).Body.String(); got != handler {
			t.Fatalf("%s served by %q, want %q", path, got, handler)
		}
	}
}
