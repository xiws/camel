package webdav

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"camel/internal/provider"
)

// Handler implements http.Handler and exposes any provider.Provider as a WebDAV server.
// Every WebDAV path is relative to "/" which maps to the provider's root dir ("0").
type Handler struct {
	p        provider.Provider
	authUser string // empty = no auth required
	authPass string

	// In-memory store for WebDAV dead properties (PROPPATCH/PROPFIND).
	propMu    sync.Mutex
	deadProps map[string]map[propName]deadProp
}

// New creates a WebDAV handler backed by the given provider.
// If authUser is non-empty, Basic auth is enforced using authPass as the password.
func New(p provider.Provider, authUser, authPass string) *Handler {
	return &Handler{
		p:         p,
		authUser:  authUser,
		authPass:  authPass,
		deadProps: make(map[string]map[propName]deadProp),
	}
}

// ---------------------------------------------------------------------------
// HTTP dispatch entry-point
// ---------------------------------------------------------------------------

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Wrap the response writer so every request can be logged with its status.
	logging := os.Getenv("CAMEL_WEBDAV_DEBUG") != ""
	rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
	w = rec
	start := time.Now()

	defer func() {
		if p := recover(); p != nil {
			w.Header().Set("Content-Type", "text/xml; charset=UTF-8")
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<D:error xmlns:D=\"DAV:\"><D:simple-error>" + escapeXML(fmt.Sprintf("internal error: %v", p)) + "</D:simple-error></D:error>"))
		}
		if logging {
			fmt.Fprintf(os.Stderr, "[webdav] %s %s depth=%q dest=%q -> %d (%d B, %s)\n",
				r.Method, r.URL.Path, r.Header.Get("Depth"), r.Header.Get("Destination"),
				rec.status, rec.bytes, time.Since(start).Round(time.Millisecond))
		}
	}()

	if h.authUser != "" && !checkAuth(r, h.authUser, h.authPass) {
		w.Header().Set("WWW-Authenticate", `Basic realm="WebDAV"`)
		http.Error(w, "401 Unauthorized", http.StatusUnauthorized)
		return
	}

	switch r.Method {
	case http.MethodOptions:
		h.handleOptions(w, r)
	case "PROPFIND":
		h.handlePropfind(w, r, r.URL.Path)
	case "PROPPATCH":
		h.handleProppatch(w, r, r.URL.Path)
	case http.MethodGet, http.MethodHead:
		h.handleGetHead(w, r, r.URL.Path)
	case http.MethodPut:
		h.handlePut(w, r, r.URL.Path)
	case http.MethodDelete:
		h.handleDelete(w, r, r.URL.Path)
	case "MKCOL":
		h.handleMkcol(w, r, r.URL.Path)
	case "MOVE":
		h.handleMove(w, r, r.URL.Path)
	case "COPY":
		h.handleCopy(w, r, r.URL.Path)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// statusRecorder wraps an http.ResponseWriter to remember the status code and
// the number of bytes written. It is used for the optional request log.
type statusRecorder struct {
	http.ResponseWriter
	status      int
	bytes       int64
	wroteHeader bool
}

func (r *statusRecorder) WriteHeader(code int) {
	if r.wroteHeader {
		return
	}
	r.status = code
	r.wroteHeader = true
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	n, err := r.ResponseWriter.Write(b)
	r.bytes += int64(n)
	return n, err
}

func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func checkAuth(r *http.Request, user, pass string) bool {
	rUser, rPass, ok := r.BasicAuth()
	return ok && rUser == user && rPass == pass
}

// normalizePath converts a WebDAV URL path to a provider-compatible string.
// "/" → "/"; "/foo/bar" → "foo/bar".
func normalizePath(webDAVPath string) string {
	p := strings.TrimPrefix(webDAVPath, "/")
	if p == "" {
		return "/"
	}
	return strings.TrimRight(p, "/")
}

// getParentAndName splits a path like "a/b/c" into parent="a/b" and name="c".
// For a bare name "c" it returns parent="" and name="c".
func getParentAndName(p string) (parent, name string) {
	parent = path.Dir(p)
	name = path.Base(p)
	if parent == "." || parent == "/" {
		return "", name
	}
	return parent, name
}

// findEntry uses List() to look up a single entry under its parent directory.
// Returns (entry, true) if found, or (nil, false) otherwise.
func findEntry(ctx context.Context, h *Handler, webDAVPath string) (*provider.FileInfo, bool) {
	relPath := normalizePath(strings.TrimSuffix(webDAVPath, "/"))
	if relPath == "" {
		return nil, false // root directory
	}
	parts := strings.Split(relPath, "/")
	parent := ""
	if len(parts) > 1 {
		parent = strings.Join(parts[:len(parts)-1], "/")
	}
	base := parts[len(parts)-1]
	if parent == "" {
		parent = "/"
	}
	items, err := h.p.List(ctx, parent)
	if err != nil {
		return nil, false
	}
	for i := range items {
		if items[i].Name == base {
			return &items[i], true
		}
	}
	return nil, false
}

// ---------------------------------------------------------------------------
// OPTIONS – advertise supported methods / capabilities
// ---------------------------------------------------------------------------

func (h *Handler) handleOptions(w http.ResponseWriter, r *http.Request) {
	// Class 1 (no locking) plus class 3 XML compliance. LOCK/UNLOCK are not
	// advertised because this server does not implement class 2 locking.
	w.Header().Set("DAV", "1, 3")
	w.Header().Set("Allow", "OPTIONS, GET, HEAD, PUT, DELETE, MKCOL, COPY, MOVE, PROPFIND, PROPPATCH")
}

// ---------------------------------------------------------------------------
// PROPFIND – return DAV properties
// ---------------------------------------------------------------------------

func (h *Handler) handlePropfind(w http.ResponseWriter, r *http.Request, urlPath string) {
	depth := r.Header.Get("Depth")

	// RFC 4918 §9.1: a server may reject Depth: infinity with 403.
	if strings.EqualFold(depth, "infinity") {
		w.Header().Set("Content-Type", "text/xml; charset=UTF-8")
		w.WriteHeader(http.StatusForbidden)
		io.WriteString(w, `<?xml version="1.0" encoding="utf-8"?><D:error xmlns:D="DAV:"><D:propfind-finite-depth/></D:error>`)
		return
	}

	body, err := readBody(r)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to read request body: %s", err), http.StatusBadRequest)
		return
	}
	req, err := parsePropfindBody(body)
	if err != nil {
		http.Error(w, fmt.Sprintf("invalid PROPFIND request body: %s", err), http.StatusBadRequest)
		return
	}

	ctx := context.Background()
	norm := normalizePath(urlPath)

	var self provider.FileInfo
	if norm == "/" {
		self = provider.FileInfo{Name: "/", IsDir: true}
	} else {
		entry, ok := findEntry(ctx, h, norm)
		if !ok {
			http.NotFound(w, r)
			return
		}
		self = *entry
	}

	var children []provider.FileInfo
	if self.IsDir && depth != "0" {
		children, err = h.p.List(ctx, norm)
		if err != nil {
			http.Error(w, fmt.Sprintf("list failed: %s", err), http.StatusInternalServerError)
			return
		}
	}

	pw := newPropWriter()
	pw.WriteString(`<D:multistatus xmlns:D="DAV:">`)
	writeResponse(pw, selfHref(urlPath, self.IsDir), h.propsFor(norm, &self, req), req.PropName)

	for i := range children {
		child := &children[i]
		childNorm := child.Name
		if norm != "/" {
			childNorm = norm + "/" + child.Name
		}
		childHref := strings.TrimRight(urlPath, "/") + "/" + child.Name
		if child.IsDir {
			childHref += "/"
		}
		writeResponse(pw, childHref, h.propsFor(childNorm, child, req), req.PropName)
	}

	pw.WriteString("</D:multistatus>")

	w.Header().Set("Content-Type", "text/xml; charset=UTF-8")
	w.WriteHeader(http.StatusMultiStatus)
	w.Write([]byte(pw.String()))
}

// selfHref returns the href for the resource a request was made against.
func selfHref(urlPath string, isDir bool) string {
	if isDir && !strings.HasSuffix(urlPath, "/") {
		return urlPath + "/"
	}
	return urlPath
}

// ---------------------------------------------------------------------------
// PROPPATCH – set/remove dead properties
// ---------------------------------------------------------------------------

func (h *Handler) handleProppatch(w http.ResponseWriter, r *http.Request, urlPath string) {
	norm := normalizePath(urlPath)
	isDir := true // the root collection always exists
	if norm != "/" {
		entry, ok := findEntry(context.Background(), h, norm)
		if !ok {
			http.NotFound(w, r)
			return
		}
		isDir = entry.IsDir
	}

	body, err := readBody(r)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to read request body: %s", err), http.StatusBadRequest)
		return
	}
	ops, err := parseProppatchBody(body)
	if err != nil {
		http.Error(w, fmt.Sprintf("invalid PROPPATCH request body: %s", err), http.StatusBadRequest)
		return
	}

	// Apply operations in document order (litmus checks remove-then-set).
	for _, op := range ops {
		if op.Remove {
			h.removeDead(norm, op.Name)
		} else {
			h.setDead(norm, op.Name, op.Value)
		}
	}

	pw := newPropWriter()
	pw.WriteString(`<D:multistatus xmlns:D="DAV:">`)
	pw.WriteString("<D:response><D:href>" + escapeXML(selfHref(urlPath, isDir)) + "</D:href>")
	pw.WriteString("<D:propstat><D:prop>")
	for _, op := range ops {
		open, close := propTagOpenClose(op.Name)
		pw.WriteString(open + close)
	}
	pw.WriteString("</D:prop><D:status>HTTP/1.1 200 OK</D:status></D:propstat></D:response>")
	pw.WriteString("</D:multistatus>")

	w.Header().Set("Content-Type", "text/xml; charset=UTF-8")
	w.WriteHeader(http.StatusMultiStatus)
	w.Write([]byte(pw.String()))
}

// ---------------------------------------------------------------------------
// GET / HEAD – stream file contents
// ---------------------------------------------------------------------------

func (h *Handler) handleGetHead(w http.ResponseWriter, r *http.Request, urlPath string) {
	entry, ok := findEntry(context.Background(), h, urlPath)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if entry.IsDir {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", mimeTypeOf(entry.Name))
	w.Header().Set("DAV", "1")
	w.Header().Set("ETag", etagFor(entry))

	// HEAD is answered from metadata; no need to download the file.
	if r.Method == http.MethodHead {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", entry.Size))
		w.WriteHeader(http.StatusOK)
		return
	}

	tmpFile, err := os.CreateTemp("", "webdav-get-*")
	if err != nil {
		http.Error(w, "500 Internal Server Error", http.StatusInternalServerError)
		return
	}
	defer func() {
		tmpFile.Close()
		os.Remove(tmpFile.Name())
	}()

	err = h.p.Download(context.Background(), normalizePath(urlPath), tmpFile.Name())
	if err != nil {
		http.Error(w, fmt.Sprintf("download failed: %s", err), http.StatusBadRequest)
		return
	}

	// ServeContent handles Content-Length, HEAD and Range requests.
	if _, err := tmpFile.Seek(0, io.SeekStart); err != nil {
		http.Error(w, "500 Internal Server Error", http.StatusInternalServerError)
		return
	}
	http.ServeContent(w, r, entry.Name, time.Time{}, tmpFile)
}

// ---------------------------------------------------------------------------
// PUT – upload content
// ---------------------------------------------------------------------------

func (h *Handler) handlePut(w http.ResponseWriter, r *http.Request, urlPath string) {
	src := r.Body
	if src == nil {
		http.Error(w, "no request body", http.StatusBadRequest)
		return
	}

	norm := normalizePath(urlPath)
	dir, fileName := getParentAndName(norm)
	if dir == "" || dir == "." {
		dir = "/"
	}
	if fileName == "" || fileName == "." || fileName == ".." || strings.ContainsAny(fileName, "/\\") {
		http.Error(w, "invalid target path", http.StatusBadRequest)
		return
	}

	ctx := context.Background()
	existed := false
	if entry, ok := findEntry(ctx, h, norm); ok {
		if entry.IsDir {
			http.Error(w, "cannot PUT to a collection", http.StatusMethodNotAllowed)
			return
		}
		existed = true
	}

	// Buffer the body in a private directory, then rename it to the exact remote
	// filename because providers derive the remote name from the local basename.
	tmpDir, err := os.MkdirTemp("", "webdav-put-*")
	if err != nil {
		http.Error(w, "500 Internal Server Error", http.StatusInternalServerError)
		return
	}
	defer os.RemoveAll(tmpDir)

	tmpFile, err := os.CreateTemp(tmpDir, "body-*")
	if err != nil {
		http.Error(w, "500 Internal Server Error", http.StatusInternalServerError)
		return
	}

	size, err := io.Copy(tmpFile, src)
	if err != nil {
		tmpFile.Close()
		http.Error(w, "failed to buffer upload", http.StatusBadRequest)
		return
	}
	if err := tmpFile.Close(); err != nil {
		http.Error(w, "500 Internal Server Error", http.StatusInternalServerError)
		return
	}

	// The provider's upload API rejects zero-byte files.
	if size == 0 {
		http.Error(w, "zero-byte uploads are not supported by the provider", http.StatusNotImplemented)
		return
	}

	tmpPath := filepath.Join(tmpDir, fileName)
	if err := os.Rename(tmpFile.Name(), tmpPath); err != nil {
		http.Error(w, "500 Internal Server Error", http.StatusInternalServerError)
		return
	}

	if err := h.p.Upload(ctx, tmpPath, normalizePath(dir)); err != nil {
		msg := strings.ToLower(err.Error())
		if strings.Contains(msg, "not found") || strings.Contains(msg, "no such file") || strings.Contains(msg, "not a directory") {
			http.Error(w, fmt.Sprintf("parent directory does not exist: %s", err), http.StatusConflict)
			return
		}
		http.Error(w, fmt.Sprintf("upload failed: %s", err), http.StatusInternalServerError)
		return
	}

	if existed {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

// ensureParentDir creates any missing parent directories for a given path.
// Only creates PARENT directories — not the final component itself.
// E.g. for "litmus/ccsrc" it creates "litmus" but NOT "litmus/ccsrc".
func (h *Handler) ensureParentDir(ctx context.Context, p string) error {
	dir := path.Dir(p)
	if dir == "" || dir == "." {
		return nil // root has no parent to create
	}
	parts := strings.Split(strings.Trim(dir, "/"), "/")
	for i := 1; i <= len(parts); i++ {
		subPath := strings.Join(parts[:i], "/")
		_, exists := findEntry(ctx, h, subPath)
		if !exists {
			if err := h.p.Mkdir(ctx, subPath); err != nil {
				msg := strings.ToLower(err.Error())
				if strings.Contains(msg, "already") {
					continue
				}
				return fmt.Errorf("mkdir parent %q failed: %w", subPath, err)
			}
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// DELETE – remove a resource
// ---------------------------------------------------------------------------

func (h *Handler) handleDelete(w http.ResponseWriter, r *http.Request, urlPath string) {
	pathArg := normalizePath(urlPath)

	// Phase 1: Verify the target exists.
	_, exists := findEntry(context.Background(), h, pathArg)
	if !exists {
		http.NotFound(w, r)
		return
	}

	// Phase 2: Delete recursively (DELETE on a collection removes its members
	// too) with retries and verification against the provider's eventual
	// consistency.
	if err := h.recursiveDelete(context.Background(), pathArg); err != nil {
		http.Error(w, fmt.Sprintf("delete failed: %s", err), http.StatusInternalServerError)
		return
	}

	h.deleteDeadProps(pathArg)
	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------------------
// MKCOL – create a directory
// ---------------------------------------------------------------------------

func (h *Handler) handleMkcol(w http.ResponseWriter, r *http.Request, urlPath string) {
	ctx := context.Background()

	// Reject request bodies with MKCOL (RFC 4918 §9.3).
	contentLength := r.ContentLength
	if contentLength > 0 || (r.Body != nil && r.Header.Get("Content-Type") != "") {
		io.Copy(io.Discard, r.Body)
		http.Error(w, "MKCOL does not accept a body", http.StatusUnsupportedMediaType)
		return
	}

	pathArg := normalizePath(urlPath)

	// Check if resource already exists — return 405 regardless of type.
	if _, ok := findEntry(ctx, h, pathArg); ok {
		http.Error(w, "resource already exists", http.StatusMethodNotAllowed)
		return
	}

	err := h.p.Mkdir(ctx, pathArg)
	if err != nil {
		msg := strings.ToLower(err.Error())
		if !strings.Contains(msg, "already") {
			http.Error(w, fmt.Sprintf("mkdir failed: %s", err), http.StatusConflict)
			return
		}
		// Already exists but wasn't found by findEntry — try again.
		if entryInfo, ok := findEntry(ctx, h, pathArg); ok && entryInfo.IsDir {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.Error(w, "resource already exists", http.StatusMethodNotAllowed)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

// ---------------------------------------------------------------------------
// MOVE – move/rename (Destination header required)
// ---------------------------------------------------------------------------

func (h *Handler) handleMove(w http.ResponseWriter, r *http.Request, urlPath string) {
	dest := r.Header.Get("Destination")
	if dest == "" {
		http.Error(w, "Missing Destination header", http.StatusBadRequest)
		return
	}

	destURL, err := url.Parse(dest)
	if err != nil {
		http.Error(w, "Bad Destination URL", http.StatusBadRequest)
		return
	}
	destPath := destURL.Path
	if !strings.HasPrefix(destPath, "/") {
		destPath = "/" + destPath
	}

	ctx := context.Background()
	srcRel := normalizePath(urlPath)
	destRel := normalizePath(destPath)

	if srcRel == destRel {
		http.Error(w, "source and destination are the same resource", http.StatusForbidden)
		return
	}

	// Check source exists.
	srcInfo, ok := findEntry(ctx, h, srcRel)
	if !ok {
		http.NotFound(w, r)
		return
	}

	// Refuse to copy/move a collection into itself.
	if strings.HasPrefix(destRel, srcRel+"/") {
		http.Error(w, "destination is inside the source collection", http.StatusConflict)
		return
	}

	destDir, destName := getParentAndName(destRel)
	if destDir == "" || destDir == "." {
		destDir = "/"
	}
	if destName == "" || destName == "." || destName == ".." {
		http.Error(w, "invalid destination path", http.StatusBadRequest)
		return
	}
	// The parent collection of the destination must already exist (RFC 4918 §9.9.4).
	if destDir != "/" {
		if _, ok := findEntry(ctx, h, destDir); !ok {
			http.Error(w, fmt.Sprintf("destination collection does not exist: %s", destDir), http.StatusConflict)
			return
		}
	}

	destExisted := false
	if _, ok := findEntry(ctx, h, destRel); ok {
		destExisted = true
	}

	// Handle Overwrite: F → MUST fail with 412 Precondition Failed if dest exists.
	if r.Header.Get("Overwrite") == "F" && destExisted {
		w.Header().Set("Content-Type", "text/xml; charset=UTF-8")
		w.WriteHeader(http.StatusPreconditionFailed)
		io.WriteString(w, `<?xml version="1.0" encoding="utf-8"?><D:error xmlns:D="DAV:"><D:precondition-failure/></D:error>`)
		return
	}

	// When Overwrite: T (or absent) and dest exists, delete it first.
	if destExisted {
		if cErr := h.recursiveDelete(ctx, destRel); cErr != nil {
			http.Error(w, fmt.Sprintf("failed to remove existing destination: %s", cErr), http.StatusInternalServerError)
			return
		}
		time.Sleep(3 * time.Second) // cloud eventual consistency delay
	}

	if srcInfo.IsDir {
		if err := h.moveDirViaProvider(ctx, srcRel, destDir, destName); err != nil {
			http.Error(w, fmt.Sprintf("move failed: %s", err), http.StatusInternalServerError)
			return
		}
	} else {
		// Use Download+Upload instead of the provider's Move: the cloud API has
		// edge cases around overwriting and eventual consistency.
		if err := h.copyFileViaDownloadUpload(ctx, srcRel, destDir, destName); err != nil {
			http.Error(w, fmt.Sprintf("move failed: %s", err), http.StatusInternalServerError)
			return
		}
		if dErr := h.recursiveDelete(ctx, srcRel); dErr != nil {
			http.Error(w, fmt.Sprintf("source cleanup failed: %s", dErr), http.StatusInternalServerError)
			return
		}
	}

	h.moveDeadProps(srcRel, destRel)
	if destExisted {
		w.WriteHeader(http.StatusNoContent)
	} else {
		w.WriteHeader(http.StatusCreated)
	}
}

// moveDirViaProvider copies a directory tree via Download+Upload, then deletes the source.
func (h *Handler) moveDirViaProvider(ctx context.Context, srcRel string, destDir string, destBase string) error {
	// Step 1: Recursively copy the source tree to destination.
	if err := h.recursiveCopyViaProvider(ctx, srcRel, destDir, destBase, "T"); err != nil {
		return err
	}
	// Step 2: Delete the original source tree (copy succeeded).
	srcNorm := normalizePath(srcRel)
	if err := h.recursiveDelete(ctx, srcNorm); err != nil {
		return fmt.Errorf("cleanup of %q after move failed: %w", srcNorm, err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// COPY – copy a resource (Destination header required)
// ---------------------------------------------------------------------------

func (h *Handler) handleCopy(w http.ResponseWriter, r *http.Request, urlPath string) {
	dest := r.Header.Get("Destination")
	if dest == "" {
		http.Error(w, "Missing Destination header", http.StatusBadRequest)
		return
	}

	destURL, err := url.Parse(dest)
	if err != nil {
		http.Error(w, "Bad Destination URL", http.StatusBadRequest)
		return
	}
	destPath := destURL.Path
	if !strings.HasPrefix(destPath, "/") {
		destPath = "/" + destPath
	}

	ctx := context.Background()
	srcRel := normalizePath(urlPath)
	destRel := normalizePath(destPath)

	if srcRel == destRel {
		http.Error(w, "source and destination are the same resource", http.StatusForbidden)
		return
	}

	// Check source exists.
	entry, ok := findEntry(ctx, h, srcRel)
	if !ok {
		http.NotFound(w, r)
		return
	}

	// Refuse to copy a collection into itself.
	if strings.HasPrefix(destRel, srcRel+"/") {
		http.Error(w, "destination is inside the source collection", http.StatusConflict)
		return
	}

	destDir, destName := getParentAndName(destRel)
	if destDir == "" || destDir == "." {
		destDir = "/"
	}
	if destName == "" || destName == "." || destName == ".." {
		http.Error(w, "invalid destination path", http.StatusBadRequest)
		return
	}
	// The parent collection of the destination must already exist (RFC 4918 §9.8.5).
	if destDir != "/" {
		if _, ok := findEntry(ctx, h, destDir); !ok {
			http.Error(w, fmt.Sprintf("destination collection does not exist: %s", destDir), http.StatusConflict)
			return
		}
	}

	destExisted := false
	if _, ok := findEntry(ctx, h, destRel); ok {
		destExisted = true
	}

	// Handle Overwrite: F → MUST fail with 412 Precondition Failed if dest exists.
	if r.Header.Get("Overwrite") == "F" && destExisted {
		w.Header().Set("Content-Type", "text/xml; charset=UTF-8")
		w.WriteHeader(http.StatusPreconditionFailed)
		io.WriteString(w, `<?xml version="1.0" encoding="utf-8"?><D:error xmlns:D="DAV:"><D:precondition-failure/></D:error>`)
		return
	}

	// When Overwrite: T (or absent) and dest exists, delete it first.
	if destExisted {
		if cErr := h.recursiveDelete(ctx, destRel); cErr != nil {
			http.Error(w, fmt.Sprintf("failed to remove existing destination: %s", cErr), http.StatusInternalServerError)
			return
		}
		time.Sleep(3 * time.Second) // cloud eventual consistency delay
	}

	if entry.IsDir {
		// Respect Depth header: 0 = just the collection itself;
		// 1 = collection + immediate children only;
		// infinity (or absent) = full recursive copy.
		depth := r.Header.Get("Depth")
		depthInt := -1 // -1 means infinity
		if depth != "" && !strings.EqualFold(depth, "infinity") {
			depthInt = 0
			fmt.Sscanf(depth, "%d", &depthInt)
		}
		if err := h.depthAwareCopy(ctx, srcRel, destDir, destName, depthInt, r.Header.Get("Overwrite")); err != nil {
			http.Error(w, fmt.Sprintf("copy failed: %s", err), http.StatusInternalServerError)
			return
		}
	} else {
		// Use Download+Upload instead of the provider's Copy: the cloud API has
		// edge cases around overwriting and eventual consistency.
		if err := h.copyFileViaDownloadUpload(ctx, srcRel, destDir, destName); err != nil {
			http.Error(w, fmt.Sprintf("copy failed: %s", err), http.StatusInternalServerError)
			return
		}
	}

	h.copyDeadProps(srcRel, destRel)
	if destExisted {
		w.WriteHeader(http.StatusNoContent)
	} else {
		w.WriteHeader(http.StatusCreated)
	}
}

// ---------------------------------------------------------------------------
// MIME type helpers
// ---------------------------------------------------------------------------

func mimeTypeOf(name string) string {
	ext := strings.ToLower(strings.TrimPrefix(path.Ext(name), "."))
	switch ext {
	case "txt":
		return "text/plain"
	case "html", "htm":
		return "text/html"
	case "css":
		return "text/css"
	case "js":
		return "application/javascript"
	case "json":
		return "application/json"
	case "pdf":
		return "application/pdf"
	case "doc":
		return "application/msword"
	case "docx":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case "xls":
		return "application/vnd.ms-excel"
	case "xlsx":
		return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	case "png":
		return "image/png"
	case "jpg", "jpeg":
		return "image/jpeg"
	case "gif":
		return "image/gif"
	case "mp3":
		return "audio/mpeg"
	case "mp4":
		return "video/mp4"
	case "zip":
		return "application/zip"
	case "gz":
		return "application/gzip"
	case "log":
		return "text/plain"
	default:
		return "application/octet-stream"
	}
}

// ---------------------------------------------------------------------------
// Recursive helpers for directory COPY / MOVE
// ---------------------------------------------------------------------------

// depthAwareCopy copies a directory tree respecting the WebDAV Depth header.
//
//	depth == 0 : create empty collection only (no children)
//	depth >= 1 : copy all immediate children (files + dirs); for dirs,
//	             recurse only if depth > 1 or unlimited (-1).
//	depth < 0  : unlimited recursion (full recursive copy).
func (h *Handler) depthAwareCopy(ctx context.Context, srcRel string, destDir string, destBase string, depth int, overwrite string) error {
	// Create destination collection — ensure parents exist first.
	destAbs := path.Join(destDir, destBase)
	if err := h.ensureParentDir(ctx, destAbs); err != nil {
		return fmt.Errorf("create parent dirs for %q failed: %w", destAbs, err)
	}
	if err := h.p.Mkdir(ctx, destAbs); err != nil {
		msg := strings.ToLower(err.Error())
		if !strings.Contains(msg, "already") {
			return fmt.Errorf("mkdir dest collection %q failed: %w", destAbs, err)
		}
	}

	// Pre-cleanup: remove ALL EXISTING children from dest that are NOT in source.
	// We must VERIFY each deletion worked because some cloud providers lie
	// (return success but keep resources alive via eventual consistency).
	srcNorm := normalizePath(srcRel)
	sourceChildren := make(map[string]bool, 0)
	srcItems, err := h.p.List(ctx, srcNorm)
	if err != nil {
		return fmt.Errorf("list source dir %q failed: %w", srcRel, err)
	}
	for _, item := range srcItems {
		sourceChildren[item.Name] = true
	}

	// Delete non-matching dest children with verification retries.
	destNormalized := normalizePath(destAbs)
	destItems, _ := h.p.List(ctx, destNormalized)
	for _, exItem := range destItems {
		if sourceChildren[exItem.Name] {
			continue // will be replaced by this copy anyway
		}
		childFull := path.Join(destNormalized, exItem.Name)
		for dTry := 0; dTry < 8; dTry++ {
			h.p.Delete(ctx, []string{childFull})
			time.Sleep(2 * time.Second)
			// Verify deletion actually took effect
			if _, present := findEntry(ctx, h, childFull); !present {
				break // successfully deleted
			}
		}
	}
	time.Sleep(2 * time.Second) // Allow final deletion to propagate.

	// Now perform the actual copy.
	for _, item := range srcItems {
		childSrcRel := path.Join(srcRel, item.Name)
		if item.IsDir {
			if depth == 0 {
				continue // skip all children when depth is 0
			}
			childDestAbs := path.Join(destAbs, item.Name)
			if err := h.p.Mkdir(ctx, childDestAbs); err != nil {
				msg := strings.ToLower(err.Error())
				if !strings.Contains(msg, "already") {
					return fmt.Errorf("mkdir subdir %q failed: %w", childDestAbs, err)
				}
			}
			// Recurse only if depth > 1 or unlimited (-1).
			if depth < 0 || depth > 1 {
				nextDepth := depth - 1
				if err := h.depthAwareCopy(ctx, childSrcRel, childDestAbs, "", nextDepth, overwrite); err != nil {
					return fmt.Errorf("copy subdir %q failed: %w", childSrcRel, err)
				}
			}
		} else {
			// Files are never recursed into, but with depth 0 copy nothing at all
			// (WebDAV Depth:0 = operation on resource only, no children).
			if depth == 0 {
				continue
			}
			// Copy file using Download+Upload.
			fileDest := path.Join(destAbs, item.Name)
			if overwrite == "F" {
				if _, exists := findEntry(ctx, h, normalizePath(fileDest)); exists {
					return fmt.Errorf("destination already exists: %s", fileDest)
				}
			}
			if exists, _ := findEntry(ctx, h, normalizePath(fileDest)); exists != nil {
				delPath := normalizePath(fileDest)
				if dErr := h.p.Delete(ctx, []string{delPath}); dErr != nil {
					msg := strings.ToLower(dErr.Error())
					if !strings.Contains(msg, "not found") && !strings.Contains(msg, "404") {
						return fmt.Errorf("delete existing dest %q failed: %w", fileDest, dErr)
					}
				}
			}
			if err := h.copyFileViaDownloadUpload(ctx, normalizePath(childSrcRel), destAbs, item.Name); err != nil {
				return fmt.Errorf("copy file %q failed: %w", childSrcRel, err)
			}
		}
	}
	return nil
}

func (h *Handler) recursiveCopyViaProvider(ctx context.Context, srcRel string, destDir string, destBase string, overwrite string) error {
	// Create destination collection — ensure parents exist first.
	destAbs := path.Join(destDir, destBase)
	if err := h.ensureParentDir(ctx, destAbs); err != nil {
		return fmt.Errorf("create parent dirs for %q failed: %w", destAbs, err)
	}
	if err := h.p.Mkdir(ctx, destAbs); err != nil {
		msg := strings.ToLower(err.Error())
		if !strings.Contains(msg, "already") {
			return fmt.Errorf("mkdir dest collection %q failed: %w", destAbs, err)
		}
	}

	// List source directory contents.
	srcNorm := normalizePath(srcRel)
	items, err := h.p.List(ctx, srcNorm)
	if err != nil {
		return fmt.Errorf("list source dir %q failed: %w", srcRel, err)
	}

	for _, item := range items {
		childSrcRel := path.Join(srcRel, item.Name)
		if item.IsDir {
			// Recurse into subdirectory.
			if err := h.recursiveCopyViaProvider(ctx, childSrcRel, destAbs, item.Name, overwrite); err != nil {
				return fmt.Errorf("copy subdir %q failed: %w", childSrcRel, err)
			}
		} else {
			// Copy individual file using Download+Upload.
			fileDest := path.Join(destAbs, item.Name)
			if overwrite == "F" {
				if _, exists := findEntry(ctx, h, normalizePath(fileDest)); exists {
					return fmt.Errorf("destination already exists: %s", fileDest)
				}
			}
			if exists, _ := findEntry(ctx, h, normalizePath(fileDest)); exists != nil {
				delPath := normalizePath(fileDest)
				if dErr := h.p.Delete(ctx, []string{delPath}); dErr != nil {
					msg := strings.ToLower(dErr.Error())
					// Tolerate not-found — might have been deleted by cloud eventuality
					if !strings.Contains(msg, "not found") && !strings.Contains(msg, "404") {
						return fmt.Errorf("delete existing dest %q failed: %w", fileDest, dErr)
					}
				}
			}
			if err := h.copyFileViaDownloadUpload(ctx, normalizePath(childSrcRel), destAbs, item.Name); err != nil {
				return fmt.Errorf("copy file %q failed: %w", childSrcRel, err)
			}
		}
	}
	return nil
}

// copyFileViaDownloadUpload downloads a file via the provider's Download method,
// then uploads it via Upload — effectively doing a GET+PUT copy.
func (h *Handler) copyFileViaDownloadUpload(ctx context.Context, srcPath, destDir, fileName string) error {
	tmpDir, err := os.MkdirTemp("", "webdav-copy-*")
	if err != nil {
		return fmt.Errorf("create temp dir failed: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	tmpPath := filepath.Join(tmpDir, fileName)
	if err := h.p.Download(ctx, srcPath, tmpPath); err != nil {
		return fmt.Errorf("download failed: %w", err)
	}

	if err := h.p.Upload(ctx, tmpPath, destDir); err != nil {
		return fmt.Errorf("upload failed: %w", err)
	}
	return nil
}

// recursiveDelete removes a resource and all its descendants.
func (h *Handler) recursiveDelete(ctx context.Context, relPath string) error {
	normalized := normalizePath(relPath)

	entry, ok := findEntry(ctx, h, normalized)
	if !ok {
		return nil // Already gone.
	}

	if entry.IsDir {
		items, err := h.p.List(ctx, normalized)
		if err != nil {
			return fmt.Errorf("list for delete failed: %w", err)
		}

		for _, item := range items {
			childRel := path.Join(normalized, item.Name)
			if dErr := h.recursiveDelete(ctx, childRel); dErr != nil {
				msg := strings.ToLower(dErr.Error())
				if !strings.Contains(msg, "not found") && !strings.Contains(msg, "404") {
					return fmt.Errorf("delete child %q failed: %w", childRel, dErr)
				}
			}
		}
		// Wait for children deletions to propagate.
		time.Sleep(1 * time.Second)
	}

	// Delete with retries AND VERIFICATION for cloud eventual consistency.
	for attempt := 0; attempt < 8; attempt++ {
		err := h.p.Delete(ctx, []string{normalized})
		if err == nil {
			// Verify the resource actually vanished.
			time.Sleep(500 * time.Millisecond)
			if _, exists := findEntry(ctx, h, normalized); !exists {
				return nil // Truly deleted.
			}
		} else {
			msg := strings.ToLower(err.Error())
			if strings.Contains(msg, "not found") || strings.Contains(msg, "404") {
				return nil // Already gone.
			}
		}
		time.Sleep(1 * time.Second)
	}
	return fmt.Errorf("delete dir %q exhausted retries — resource may be ghosted by cloud provider", normalized)
}

// ---------------------------------------------------------------------------
// XML helper
// ---------------------------------------------------------------------------

// propWriter accumulates XML bytes for a PROPFIND response.
type propWriter struct {
	strings.Builder
}

func newPropWriter() *propWriter {
	return &propWriter{}
}

func escapeXML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, `"`, "&quot;")
	return s
}
