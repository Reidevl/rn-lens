package wailsui

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
)

// maxUploadBytes bounds one upload; it matches the storage import limit.
const maxUploadBytes = maxImportBytes

// maxPendingUploads bounds the uploads kept for an import; older ones are
// removed, so a page cannot fill the disk.
const maxPendingUploads = 4

// maxPendingDownloads bounds the saves the browser has not fetched yet;
// older ones are removed.
const maxPendingDownloads = 16

// errUnknownTransfer means a download or upload token is not (or no longer)
// registered.
var errUnknownTransfer = errors.New("lens: unknown or expired transfer")

// transfers replaces native file dialogs in browser mode: a save becomes a
// file the browser downloads once, an open becomes a file the browser
// uploaded first. Files live in a private temporary directory and are
// reachable only through random tokens.
type transfers struct {
	dir string

	mu        sync.Mutex
	downloads map[string]string // token -> file path
	byPath    map[string]string // file path -> token
	uploads   map[string]string // token -> file path
	upOrder   []string          // upload tokens, oldest first
	downOrder []string          // download tokens, oldest first
}

func newTransfers() (*transfers, error) {
	dir, err := os.MkdirTemp("", "lens-transfers-")
	if err != nil {
		return nil, fmt.Errorf("lens: transfers: %w", err)
	}
	return &transfers{dir: dir, downloads: map[string]string{}, byPath: map[string]string{}, uploads: map[string]string{}}, nil
}

// close removes every pending file.
func (t *transfers) close() { _ = os.RemoveAll(t.dir) }

func token() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// savePath is a SaveAsFunc for browser mode: it returns a fresh path named
// name that the browser downloads once the service has written it.
func (t *transfers) savePath(name string) (string, error) {
	tok := token()
	dir := filepath.Join(t.dir, tok)
	if err := os.Mkdir(dir, 0o700); err != nil {
		return "", fmt.Errorf("lens: transfers: %w", err)
	}
	path := filepath.Join(dir, filepath.Base(name))
	t.mu.Lock()
	t.downloads[tok] = path
	t.byPath[path] = tok
	t.downOrder = append(t.downOrder, tok)
	var evicted []string
	for len(t.downOrder) > maxPendingDownloads {
		old := t.downOrder[0]
		t.downOrder = t.downOrder[1:]
		if p, ok := t.downloads[old]; ok {
			evicted = append(evicted, filepath.Dir(p))
			delete(t.downloads, old)
			delete(t.byPath, p)
		}
	}
	t.mu.Unlock()
	for _, dir := range evicted {
		_ = os.RemoveAll(dir)
	}
	return path, nil
}

// downloadURL returns the one-time URL of a path savePath returned, or ""
// for any other path.
func (t *transfers) downloadURL(path string) string {
	t.mu.Lock()
	defer t.mu.Unlock()
	tok, ok := t.byPath[path]
	if !ok {
		return ""
	}
	return "/lens/downloads/" + tok + "/" + filepath.Base(path)
}

// uploadPath returns the file an upload stored, or errUnknownTransfer.
func (t *transfers) uploadPath(tok string) (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	path, ok := t.uploads[tok]
	if !ok {
		return "", errUnknownTransfer
	}
	return path, nil
}

// forgetUpload removes an upload once it was used.
func (t *transfers) forgetUpload(tok string) {
	t.mu.Lock()
	path, ok := t.uploads[tok]
	delete(t.uploads, tok)
	t.upOrder = slices.DeleteFunc(t.upOrder, func(s string) bool { return s == tok })
	t.mu.Unlock()
	if ok {
		_ = os.RemoveAll(filepath.Dir(path))
	}
}

// ServeHTTP handles GET /lens/downloads/<token>/<name> (once, then the file
// is deleted) and POST /lens/uploads (the body is the file; the reply is
// {"token": ...}).
func (t *transfers) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/lens/downloads/"):
		t.serveDownload(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/lens/uploads":
		if !sameOrigin(r) {
			http.Error(w, "uploads are only accepted from the Lens page", http.StatusForbidden)
			return
		}
		t.receiveUpload(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (t *transfers) serveDownload(w http.ResponseWriter, r *http.Request) {
	tok, _, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/lens/downloads/"), "/")
	t.mu.Lock()
	path, ok := t.downloads[tok]
	delete(t.downloads, tok)
	delete(t.byPath, path)
	t.downOrder = slices.DeleteFunc(t.downOrder, func(s string) bool { return s == tok })
	t.mu.Unlock()
	if !ok {
		http.NotFound(w, r)
		return
	}
	defer func() { _ = os.RemoveAll(filepath.Dir(path)) }()
	f, err := os.Open(path) //nolint:gosec // path comes from savePath, never from the request
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer func() { _ = f.Close() }()
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": filepath.Base(path)}))
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = io.Copy(w, f)
}

func (t *transfers) receiveUpload(w http.ResponseWriter, r *http.Request) {
	tok := token()
	dir := filepath.Join(t.dir, "up-"+tok)
	if err := os.Mkdir(dir, 0o700); err != nil {
		http.Error(w, "cannot store the upload", http.StatusInternalServerError)
		return
	}
	path := filepath.Join(dir, "upload")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o600) //nolint:gosec // fixed name in a private directory
	if err != nil {
		http.Error(w, "cannot store the upload", http.StatusInternalServerError)
		return
	}
	n, err := io.Copy(f, io.LimitReader(r.Body, maxUploadBytes+1))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil || n > maxUploadBytes {
		_ = os.RemoveAll(dir)
		http.Error(w, fmt.Sprintf("the file must be at most %d MiB", maxUploadBytes>>20), http.StatusRequestEntityTooLarge)
		return
	}
	t.mu.Lock()
	t.uploads[tok] = path
	t.upOrder = append(t.upOrder, tok)
	var evicted []string
	for len(t.upOrder) > maxPendingUploads {
		old := t.upOrder[0]
		t.upOrder = t.upOrder[1:]
		if p, ok := t.uploads[old]; ok {
			evicted = append(evicted, filepath.Dir(p))
			delete(t.uploads, old)
		}
	}
	t.mu.Unlock()
	for _, dir := range evicted {
		_ = os.RemoveAll(dir)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"token": tok})
}

// sameOrigin reports whether a request comes from the Lens page itself. A
// page the user opened on another site can POST to localhost too: browsers
// mark it with Sec-Fetch-Site and an Origin that is not Lens's host.
func sameOrigin(r *http.Request) bool {
	switch r.Header.Get("Sec-Fetch-Site") {
	case "cross-site", "same-site":
		return false
	}
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true // not a browser cross-origin request (curl, the tests)
	}
	u, err := url.Parse(origin)
	return err == nil && strings.EqualFold(u.Host, r.Host)
}
