package webdav

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"

	"camel/internal/provider"
)

// Handler implements http.Handler and exposes any provider.Provider as a WebDAV server.
// Every WebDAV path is relative to "/" which maps to the provider's root dir ("0").
type Handler struct {
	p        provider.Provider
	authUser string // empty = no auth required
	authPass string
}

// New creates a WebDAV handler backed by the given provider.
// If authUser is non-empty, Basic auth is enforced using authPass as the password.
func New(p provider.Provider, authUser, authPass string) *Handler {
	return &Handler{p: p, authUser: authUser, authPass: authPass}
}

// ---------------------------------------------------------------------------
// HTTP dispatch entry-point
// ---------------------------------------------------------------------------

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if rec := recover(); rec != nil {
			w.Header().Set("Content-Type", "text/xml; charset=UTF-8")
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<D:error xmlns:D=\"DAV:\"><D:simple-error>" + escapeXML(fmt.Sprintf("internal error: %v", rec)) + "</D:simple-error></D:error>"))
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
	return p
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
	base := path.Base(webDAVPath)
	items, err := h.p.List(ctx, relPath)
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

func parseDestination(dest string) (string, error) {
	u, err := url.Parse(dest)
	if err != nil {
		return "", fmt.Errorf("invalid destination URL %q: %w", dest, err)
	}
	if u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("destination must be an absolute URL")
	}
	return u.Path, nil
}

// ---------------------------------------------------------------------------
// OPTIONS – advertise supported methods / capabilities
// ---------------------------------------------------------------------------

func (h *Handler) handleOptions(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("DAV", "1, 3")
	w.Header().Set("Allow", "OPTIONS, GET, HEAD, PUT, DELETE, MKCOL, COPY, MOVE, PROPFIND")
}

// ---------------------------------------------------------------------------
// PROPFIND – return DAV properties
// ---------------------------------------------------------------------------

func (h *Handler) handlePropfind(w http.ResponseWriter, r *http.Request, urlPath string) {
	depth := r.Header.Get("Depth")

	// Read and discard optional <prop> element from request body
	_, _ = io.Copy(io.Discard, r.Body)

	pw := newPropWriter()
	pw.WriteString(`<multistatus xmlns="urn:ietf:params:xml:ns:webdav">`)

	// Self resource
	writeSelfResponse(pw, urlPath)

	listDir := normalizePath(urlPath)
	if depth != "0" && depth != "" {
		ctx := context.Background()
		items, err := h.p.List(ctx, listDir)
		if err != nil {
			writeDAVError(w, http.StatusInternalServerError, fmt.Sprintf("list failed: %s", err))
			return
		}
		for _, item := range items {
			childRelPath := strings.TrimRight(urlPath, "/") + "/" + item.Name
			writeChildResponse(pw, childRelPath+"/", item.IsDir, item.Name, item.Size)
		}
	}

	pw.WriteString("</multistatus>")

	w.Header().Set("Content-Type", "text/xml; charset=UTF-8")
	w.WriteHeader(207) // MultiStatus
	w.Write([]byte(pw.Builder.String()))
}

func writeSelfResponse(pw *propWriter, href string) {
	pw.WriteString("<response><href>" + escapeXML(href) + "</href><propstat>")
	pw.WriteString("<prop>")
	if href == "/" || strings.HasSuffix(href, "/") {
		pw.WriteString(`<resourcetype><collection/></resourcetype>`)
		pw.WriteString(`<displayname>/</displayname>`)
	} else {
		pw.WriteString(`<resourcetype></resourcetype>`)
		pw.WriteString(`<getcontentlength>unknown</getcontentlength>`)
		pw.WriteString(`<getcontenttype>application/octet-stream</getcontenttype>`)
		pw.WriteString(`<displayname>` + escapeXML(path.Base(href)) + `</displayname>`)
	}
	pw.WriteString("</prop><status>HTTP/1.1 200 OK</status></propstat></response>")
}

func writeChildResponse(pw *propWriter, href string, isDir bool, name string, size int64) {
	pw.WriteString("<response><href>" + escapeXML(href) + "</href><propstat>")
	pw.WriteString("<prop>")
	if isDir {
		pw.WriteString(`<resourcetype><collection/></resourcetype>`)
		pw.WriteString(`<displayname>` + escapeXML(name) + `</displayname>`)
	} else {
		pw.WriteString(`<resourcetype></resourcetype>`)
		pw.WriteString(`<getcontentlength>` + fmt.Sprintf("%d", size) + `</getcontentlength>`)
		pw.WriteString(`<getcontenttype>` + mimeTypeOf(name) + `</getcontenttype>`)
		pw.WriteString(`<displayname>` + escapeXML(name) + `</displayname>`)
	}
	pw.WriteString("</prop><status>HTTP/1.1 200 OK</status></propstat></response>")
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

	info, _ := tmpFile.Stat()
	w.Header().Set("Content-Length", fmt.Sprintf("%d", info.Size()))
	w.Header().Set("Content-Type", mimeTypeOf(entry.Name))
	w.Header().Set("DAV", "1")
	w.Header().Set("ETag", fmt.Sprintf("\"%x\"", info.Size()))

	if r.Method == http.MethodHead {
		return
	}

	tmpFile.Seek(0, 0)
	io.Copy(w, tmpFile)
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

	tmpFile, err := os.CreateTemp("", "webdav-put-*")
	if err != nil {
		http.Error(w, "500 Internal Server Error", http.StatusInternalServerError)
		return
	}
	defer func() {
		tmpFile.Close()
		os.Remove(tmpFile.Name())
	}()

	size, err := io.Copy(tmpFile, src)
	if err != nil {
		http.Error(w, "failed to buffer upload", http.StatusBadRequest)
		return
	}

	if size == 0 {
		http.Error(w, "zero-byte uploads are not supported", http.StatusNotImplemented)
		return
	}

	tmpFile.Close()

	dir, _ := getParentAndName(normalizePath(urlPath))
	if dir == "" {
		dir = "/"
	}

	err = h.p.Upload(context.Background(), tmpFile.Name(), dir)
	if err != nil {
		http.Error(w, fmt.Sprintf("upload failed: %s", err), http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

// ---------------------------------------------------------------------------
// DELETE – remove a resource
// ---------------------------------------------------------------------------

func (h *Handler) handleDelete(w http.ResponseWriter, r *http.Request, urlPath string) {
	pathArg := normalizePath(urlPath)
	err := h.p.Delete(context.Background(), []string{pathArg})
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------------------
// MKCOL – create a directory
// ---------------------------------------------------------------------------

func (h *Handler) handleMkcol(w http.ResponseWriter, r *http.Request, urlPath string) {
	pathArg := normalizePath(urlPath)
	err := h.p.Mkdir(context.Background(), pathArg)
	if err != nil {
		http.Error(w, fmt.Sprintf("mkdir failed: %s", err), http.StatusConflict)
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

	destPath, err := parseDestination(dest)
	if err != nil {
		http.Error(w, "Bad Destination URL", http.StatusBadRequest)
		return
	}

	srcPath := normalizePath(urlPath)
	destProviderPath := normalizePath(destPath)

	dir, newName := getParentAndName(destProviderPath)
	if dir == "" {
		dir = "/"
	}

	err = h.p.Move(context.Background(), srcPath, dir, newName)
	if err != nil {
		http.Error(w, fmt.Sprintf("move failed: %s", err), http.StatusForbidden)
		return
	}
	w.WriteHeader(http.StatusNoContent)
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

	destPath, err := parseDestination(dest)
	if err != nil {
		http.Error(w, "Bad Destination URL", http.StatusBadRequest)
		return
	}

	srcPath := normalizePath(urlPath)
	destProviderPath := normalizePath(destPath)

	dir, newName := getParentAndName(destProviderPath)
	if dir == "" {
		dir = "/"
	}

	err = h.p.Copy(context.Background(), srcPath, dir, newName)
	if err != nil {
		http.Error(w, fmt.Sprintf("copy failed: %s", err), http.StatusForbidden)
		return
	}
	w.WriteHeader(http.StatusCreated)
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

func writeDAVError(w http.ResponseWriter, status int, desc string) {
	resp := "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n" +
		"<D:error xmlns:D=\"DAV:\">\n" +
		"<D:simple-error>\n" +
		escapeXML(desc) + "\n" +
		"</D:simple-error>\n" +
		"</D:error>"

	w.Header().Set("Content-Type", "text/xml; charset=UTF-8")
	w.WriteHeader(status)
	w.Write([]byte(resp))
}
