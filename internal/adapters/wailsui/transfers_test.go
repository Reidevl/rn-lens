package wailsui

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func newTestTransfers(t *testing.T) *transfers {
	t.Helper()
	tr, err := newTransfers()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tr.close)
	return tr
}

func TestADownloadIsServedOnceAsAnAttachment(t *testing.T) {
	t.Parallel()
	tr := newTestTransfers(t)
	path, err := tr.savePath("lens-trace.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"traceEvents":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	url := tr.downloadURL(path)
	if !strings.HasPrefix(url, "/lens/downloads/") || !strings.HasSuffix(url, "/lens-trace.json") {
		t.Fatalf("downloadURL = %q", url)
	}
	if tr.downloadURL("/etc/passwd") != "" {
		t.Fatal("a path savePath did not create got a URL")
	}

	rec := httptest.NewRecorder()
	tr.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, url, http.NoBody))
	if rec.Code != http.StatusOK || rec.Body.String() != `{"traceEvents":[]}` {
		t.Fatalf("download = %d %q", rec.Code, rec.Body.String())
	}
	if cd := rec.Header().Get("Content-Disposition"); cd != `attachment; filename=lens-trace.json` {
		t.Fatalf("Content-Disposition = %q", cd)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("the file is still on disk after the download: %v", err)
	}
	rec = httptest.NewRecorder()
	tr.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, url, http.NoBody))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("second download = %d, want 404", rec.Code)
	}
}

func TestUnknownTransfersAreRefused(t *testing.T) {
	t.Parallel()
	tr := newTestTransfers(t)
	for _, req := range []*http.Request{
		httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/lens/downloads/nope/x.json", http.NoBody),
		httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/lens/downloads/../../etc/passwd", http.NoBody),
		httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/lens/uploads", http.NoBody),
	} {
		rec := httptest.NewRecorder()
		tr.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s %s = %d, want 404", req.Method, req.URL.Path, rec.Code)
		}
	}
	if _, err := tr.uploadPath("nope"); !errors.Is(err, errUnknownTransfer) {
		t.Fatalf("uploadPath(nope) err = %v", err)
	}
}

func TestAnUploadIsStoredUntilUsed(t *testing.T) {
	t.Parallel()
	tr := newTestTransfers(t)
	rec := httptest.NewRecorder()
	tr.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/lens/uploads", strings.NewReader(`{"format":"x"}`)))
	var reply struct{ Token string }
	if err := json.Unmarshal(rec.Body.Bytes(), &reply); err != nil || rec.Code != http.StatusOK || reply.Token == "" {
		t.Fatalf("upload = %d %s", rec.Code, rec.Body.String())
	}
	path, err := tr.uploadPath(reply.Token)
	if err != nil {
		t.Fatal(err)
	}
	f, _ := os.Open(path)
	got, _ := io.ReadAll(f)
	_ = f.Close()
	if string(got) != `{"format":"x"}` {
		t.Fatalf("stored %q", got)
	}
	tr.forgetUpload(reply.Token)
	if _, err := tr.uploadPath(reply.Token); !errors.Is(err, errUnknownTransfer) {
		t.Fatal("the upload is still registered after use")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("the upload is still on disk after use")
	}
}

func TestOversizedUploadsAreRejected(t *testing.T) {
	t.Parallel()
	tr := newTestTransfers(t)
	rec := httptest.NewRecorder()
	tr.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/lens/uploads", io.LimitReader(zeros{}, maxUploadBytes+10)))
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("upload = %d, want 413", rec.Code)
	}
	entries, _ := os.ReadDir(tr.dir)
	if len(entries) != 0 {
		t.Fatalf("leftovers after a rejected upload: %v", entries)
	}
}

type zeros struct{}

func (zeros) Read(p []byte) (int, error) {
	clear(p)
	return len(p), nil
}

// Any page open in the browser can POST to localhost; only Lens's own page
// may upload.
func TestUploadRejectsCrossOriginRequests(t *testing.T) {
	t.Parallel()
	tr := newTestTransfers(t)
	upload := func(set func(*http.Request)) int {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/lens/uploads", strings.NewReader("data"))
		req.Host = "localhost:7430"
		set(req)
		rec := httptest.NewRecorder()
		tr.ServeHTTP(rec, req)
		return rec.Code
	}
	for name, set := range map[string]func(*http.Request){
		"foreign Origin":        func(r *http.Request) { r.Header.Set("Origin", "https://evil.example") },
		"other localhost port":  func(r *http.Request) { r.Header.Set("Origin", "http://localhost:3000") },
		"cross-site, no Origin": func(r *http.Request) { r.Header.Set("Sec-Fetch-Site", "cross-site") },
		"same-site, no Origin":  func(r *http.Request) { r.Header.Set("Sec-Fetch-Site", "same-site") },
	} {
		if code := upload(set); code != http.StatusForbidden {
			t.Errorf("%s: status %d, want 403", name, code)
		}
	}
	if entries, _ := os.ReadDir(tr.dir); len(entries) != 0 {
		t.Fatalf("cross-origin uploads stored %d files", len(entries))
	}
	if code := upload(func(r *http.Request) {
		r.Header.Set("Origin", "http://localhost:7430")
		r.Header.Set("Sec-Fetch-Site", "same-origin")
	}); code != http.StatusOK {
		t.Fatalf("same-origin upload: status %d", code)
	}
}

func TestOnlyTheLatestUploadsAreKept(t *testing.T) {
	t.Parallel()
	tr := newTestTransfers(t)
	tokens := make([]string, 0, maxPendingUploads+2)
	for range maxPendingUploads + 2 {
		rec := httptest.NewRecorder()
		tr.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/lens/uploads", strings.NewReader("{}")))
		var reply struct{ Token string }
		if err := json.Unmarshal(rec.Body.Bytes(), &reply); err != nil {
			t.Fatal(err)
		}
		tokens = append(tokens, reply.Token)
	}
	for i, tok := range tokens {
		_, err := tr.uploadPath(tok)
		if kept := i >= 2; kept != (err == nil) {
			t.Fatalf("upload %d: kept=%v err=%v", i, kept, err)
		}
	}
	if entries, _ := os.ReadDir(tr.dir); len(entries) != maxPendingUploads {
		t.Fatalf("%d uploads on disk, want %d", len(entries), maxPendingUploads)
	}
}

func TestOnlyTheLatestDownloadsAreKept(t *testing.T) {
	t.Parallel()
	tr := newTestTransfers(t)
	paths := make([]string, 0, maxPendingDownloads+2)
	for range maxPendingDownloads + 2 {
		path, err := tr.savePath("lens.har")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, path)
	}
	for i, path := range paths {
		_, statErr := os.Stat(path)
		if kept := i >= 2; kept != (tr.downloadURL(path) != "") || kept != (statErr == nil) {
			t.Fatalf("download %d: kept=%v url=%q stat=%v", i, kept, tr.downloadURL(path), statErr)
		}
	}
}
