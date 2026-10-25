package webdav

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"hash/fnv"
	"io"
	"net/http"
	"strings"
	"time"

	"camel/internal/provider"
)

// This file implements the WebDAV "dead property" store used by PROPPATCH and
// the property filtering used by PROPFIND. Dead properties are kept in memory
// only: they survive for the lifetime of the server process and are carried
// across MOVE/COPY/DELETE within that process.

const davNS = "DAV:"

type propName struct {
	Space string
	Local string
}

type deadProp struct {
	Text string // character data content (simple properties)
	XML  string // serialized child elements (structured properties)
}

type propOut struct {
	Name   propName
	Value  string // raw XML fragment used as the element content
	Status int    // 200 or 404
}

// ---------------------------------------------------------------------------
// Dead property store
// ---------------------------------------------------------------------------

func (h *Handler) deadFor(path string) map[propName]deadProp {
	h.propMu.Lock()
	defer h.propMu.Unlock()
	src := h.deadProps[path]
	if len(src) == 0 {
		return nil
	}
	out := make(map[propName]deadProp, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

func (h *Handler) setDead(path string, name propName, value deadProp) {
	h.propMu.Lock()
	defer h.propMu.Unlock()
	if h.deadProps == nil {
		h.deadProps = make(map[string]map[propName]deadProp)
	}
	m := h.deadProps[path]
	if m == nil {
		m = make(map[propName]deadProp)
		h.deadProps[path] = m
	}
	m[name] = value
}

func (h *Handler) removeDead(path string, name propName) {
	h.propMu.Lock()
	defer h.propMu.Unlock()
	if m := h.deadProps[path]; m != nil {
		delete(m, name)
		if len(m) == 0 {
			delete(h.deadProps, path)
		}
	}
}

// deleteDeadProps removes properties for path and any descendants.
func (h *Handler) deleteDeadProps(path string) {
	h.propMu.Lock()
	defer h.propMu.Unlock()
	delete(h.deadProps, path)
	prefix := strings.TrimSuffix(path, "/") + "/"
	for p := range h.deadProps {
		if strings.HasPrefix(p, prefix) {
			delete(h.deadProps, p)
		}
	}
}

// copyDeadProps copies properties from src to dst, including descendants.
func (h *Handler) copyDeadProps(src, dst string) {
	h.propMu.Lock()
	defer h.propMu.Unlock()
	copied := make(map[string]map[propName]deadProp)
	for p, m := range h.deadProps {
		var target string
		switch {
		case p == src:
			target = dst
		case strings.HasPrefix(p, strings.TrimSuffix(src, "/")+"/"):
			target = dst + strings.TrimPrefix(p, src)
		default:
			continue
		}
		clone := make(map[propName]deadProp, len(m))
		for k, v := range m {
			clone[k] = v
		}
		copied[target] = clone
	}
	for p, m := range copied {
		h.deadProps[p] = m
	}
}

// moveDeadProps moves properties from src to dst, including descendants.
func (h *Handler) moveDeadProps(src, dst string) {
	h.propMu.Lock()
	defer h.propMu.Unlock()
	moved := make(map[string]map[propName]deadProp)
	for p, m := range h.deadProps {
		var target string
		switch {
		case p == src:
			target = dst
		case strings.HasPrefix(p, strings.TrimSuffix(src, "/")+"/"):
			target = dst + strings.TrimPrefix(p, src)
		default:
			continue
		}
		moved[target] = m
		delete(h.deadProps, p)
	}
	for p, m := range moved {
		h.deadProps[p] = m
	}
}

// ---------------------------------------------------------------------------
// Request parsing
// ---------------------------------------------------------------------------

// rejectInvalidNSDecls catches the namespace declaration litmus considers
// invalid (an empty URI for a non-empty prefix, e.g. xmlns:bar="").
func rejectInvalidNSDecls(body []byte) error {
	s := string(body)
	for {
		i := strings.Index(s, "xmlns:")
		if i < 0 {
			return nil
		}
		rest := s[i+len("xmlns:"):]
		j := strings.IndexAny(rest, "= \t\r\n")
		if j < 0 {
			return nil
		}
		after := strings.TrimLeft(rest[j:], " \t\r\n")
		if strings.HasPrefix(after, "=\"\"") || strings.HasPrefix(after, "=''") {
			return fmt.Errorf("invalid namespace declaration: empty URI for prefix")
		}
		s = rest
	}
}

type propfindReq struct {
	AllProp  bool
	PropName bool
	Names    []propName
	Include  []propName
}

func parsePropfindBody(body []byte) (propfindReq, error) {
	var req propfindReq

	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		req.AllProp = true
		return req, nil
	}
	if err := rejectInvalidNSDecls(trimmed); err != nil {
		return req, err
	}

	dec := xml.NewDecoder(bytes.NewReader(trimmed))
	root, err := skipToRoot(dec, "propfind")
	if err != nil {
		return req, err
	}

	depth := 1
	for depth > 0 {
		tok, err := dec.Token()
		if err != nil {
			return req, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if depth == 1 {
				switch t.Name.Local {
				case "allprop":
					req.AllProp = true
					depth++
					continue
				case "propname":
					req.PropName = true
					depth++
					continue
				case "prop", "include":
					names, err := parsePropNames(dec)
					if err != nil {
						return req, err
					}
					if t.Name.Local == "include" {
						req.Include = append(req.Include, names...)
					} else {
						req.Names = append(req.Names, names...)
					}
					continue
				}
			}
			depth++
		case xml.EndElement:
			depth--
		}
	}

	if !req.AllProp && !req.PropName && len(req.Names) == 0 {
		// An empty <propfind/> is equivalent to allprop for compatibility.
		req.AllProp = true
	}
	_ = root
	return req, nil
}

func skipToRoot(dec *xml.Decoder, local string) (xml.StartElement, error) {
	for {
		tok, err := dec.Token()
		if err != nil {
			return xml.StartElement{}, err
		}
		if se, ok := tok.(xml.StartElement); ok {
			if se.Name.Local != local {
				return xml.StartElement{}, fmt.Errorf("expected <%s>, got <%s>", local, se.Name.Local)
			}
			return se, nil
		}
	}
}

// parsePropNames consumes a <prop> or <include> element and returns the names
// of its direct child elements.
func parsePropNames(dec *xml.Decoder) ([]propName, error) {
	names := make([]propName, 0, 8)
	depth := 1
	for depth > 0 {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if depth == 1 {
				names = append(names, propName{Space: t.Name.Space, Local: t.Name.Local})
			}
			depth++
		case xml.EndElement:
			depth--
		}
	}
	return names, nil
}

type patchOp struct {
	Remove bool
	Name   propName
	Value  deadProp
}

func parseProppatchBody(body []byte) ([]patchOp, error) {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("empty PROPPATCH body")
	}
	if err := rejectInvalidNSDecls(trimmed); err != nil {
		return nil, err
	}

	dec := xml.NewDecoder(bytes.NewReader(trimmed))
	if _, err := skipToRoot(dec, "propertyupdate"); err != nil {
		return nil, err
	}

	var ops []patchOp
	depth := 1
	remove := false
	for depth > 0 {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if depth == 1 {
				switch t.Name.Local {
				case "set":
					remove = false
				case "remove":
					remove = true
				}
				depth++
				continue
			}
			if depth == 2 && t.Name.Local == "prop" {
				props, err := parsePatchProps(dec, remove)
				if err != nil {
					return nil, err
				}
				ops = append(ops, props...)
				continue
			}
			depth++
		case xml.EndElement:
			depth--
		}
	}
	return ops, nil
}

// parsePatchProps consumes the contents of a <prop> element inside
// PROPPATCH and returns one operation per child property, in document order.
func parsePatchProps(dec *xml.Decoder, remove bool) ([]patchOp, error) {
	var ops []patchOp
	depth := 1
	for depth > 0 {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if depth == 1 {
				value, err := parsePropValue(dec)
				if err != nil {
					return nil, err
				}
				ops = append(ops, patchOp{
					Remove: remove,
					Name:   propName{Space: t.Name.Space, Local: t.Name.Local},
					Value:  value,
				})
				continue
			}
			depth++
		case xml.EndElement:
			depth--
		}
	}
	return ops, nil
}

// parsePropValue consumes a property element and captures either its text
// content or its nested XML content.
func parsePropValue(dec *xml.Decoder) (deadProp, error) {
	var text strings.Builder
	var buf bytes.Buffer
	enc := xml.NewEncoder(&buf)
	hasChild := false
	depth := 0

	for {
		tok, err := dec.Token()
		if err != nil {
			return deadProp{}, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			hasChild = true
			depth++
			// Namespace declarations are re-synthesized by the encoder from
			// Name.Space; keeping the decoded xmlns attributes would emit
			// duplicate xmlns attributes (which is invalid XML).
			t.Attr = stripNSAttrs(t.Attr)
			if err := enc.EncodeToken(t); err != nil {
				return deadProp{}, err
			}
		case xml.EndElement:
			if depth == 0 {
				if hasChild {
					if err := enc.Flush(); err != nil {
						return deadProp{}, err
					}
					return deadProp{XML: buf.String()}, nil
				}
				return deadProp{Text: text.String()}, nil
			}
			depth--
			if err := enc.EncodeToken(t); err != nil {
				return deadProp{}, err
			}
		case xml.CharData:
			if depth == 0 && !hasChild {
				text.Write(t)
			} else {
				if err := enc.EncodeToken(t); err != nil {
					return deadProp{}, err
				}
			}
		case xml.Comment, xml.ProcInst, xml.Directive:
			// Ignore.
		}
	}
}

// stripNSAttrs removes xmlns declarations from a decoded start element.
func stripNSAttrs(attrs []xml.Attr) []xml.Attr {
	if len(attrs) == 0 {
		return attrs
	}
	out := attrs[:0]
	for _, a := range attrs {
		if a.Name.Local == "xmlns" && a.Name.Space == "" {
			continue
		}
		out = append(out, a)
	}
	return out
}

// ---------------------------------------------------------------------------
// Response building
// ---------------------------------------------------------------------------

// liveProps returns the DAV: live properties for a resource. Values are raw
// XML fragments (already escaped where they are text).
func liveProps(entry *provider.FileInfo) map[propName]string {
	now := time.Now().UTC()
	props := map[propName]string{
		{Space: davNS, Local: "displayname"}:     escapeXML(entry.Name),
		{Space: davNS, Local: "getlastmodified"}: now.Format(http.TimeFormat),
		{Space: davNS, Local: "creationdate"}:    now.Format(time.RFC3339),
	}
	if entry.IsDir {
		props[propName{Space: davNS, Local: "resourcetype"}] = "<D:collection/>"
	} else {
		props[propName{Space: davNS, Local: "resourcetype"}] = ""
		props[propName{Space: davNS, Local: "getcontentlength"}] = fmt.Sprintf("%d", entry.Size)
		props[propName{Space: davNS, Local: "getcontenttype"}] = escapeXML(mimeTypeOf(entry.Name))
		props[propName{Space: davNS, Local: "getetag"}] = escapeXML(etagFor(entry))
	}
	return props
}

// etagFor returns a stable entity tag derived from the resource name and size.
func etagFor(entry *provider.FileInfo) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(entry.Name))
	return fmt.Sprintf(`"%x-%x"`, entry.Size, h.Sum32())
}

// propsFor builds the property results for one resource according to the
// PROPFIND request. Missing properties are reported with status 404 instead of
// being omitted, as required by RFC 4918.
func (h *Handler) propsFor(path string, entry *provider.FileInfo, req propfindReq) []propOut {
	live := liveProps(entry)
	dead := h.deadFor(path)

	if req.PropName {
		out := make([]propOut, 0, len(live)+len(dead))
		for name := range live {
			out = append(out, propOut{Name: name, Status: http.StatusOK})
		}
		for name := range dead {
			out = append(out, propOut{Name: name, Status: http.StatusOK})
		}
		return out
	}

	names := req.Names
	all := req.AllProp
	if all {
		names = make([]propName, 0, len(live)+len(dead))
		for name := range live {
			names = append(names, name)
		}
		for name := range dead {
			names = append(names, name)
		}
		names = append(names, req.Include...)
	}

	seen := make(map[propName]bool, len(names))
	out := make([]propOut, 0, len(names))
	for _, name := range names {
		if seen[name] {
			continue
		}
		seen[name] = true
		if v, ok := live[name]; ok {
			out = append(out, propOut{Name: name, Value: v, Status: http.StatusOK})
			continue
		}
		if dp, ok := dead[name]; ok {
			out = append(out, propOut{Name: name, Value: deadPropValue(dp), Status: http.StatusOK})
			continue
		}
		out = append(out, propOut{Name: name, Status: http.StatusNotFound})
	}
	return out
}

func deadPropValue(dp deadProp) string {
	if dp.XML != "" {
		return dp.XML
	}
	return escapeXML(dp.Text)
}

// propTagOpen returns the opening tag for a property element.
func propTagOpen(name propName) string {
	switch name.Space {
	case davNS:
		return "<D:" + name.Local + ">"
	case "":
		return "<" + name.Local + ` xmlns="">`
	default:
		return "<" + name.Local + ` xmlns="` + escapeXML(name.Space) + `">`
	}
}

func propTagOpenClose(name propName) (string, string) {
	open := propTagOpen(name)
	if name.Space == davNS {
		return open, "</D:" + name.Local + ">"
	}
	return open, "</" + name.Local + ">"
}

func writePropOut(pw *propWriter, p propOut) {
	open, close := propTagOpenClose(p.Name)
	pw.WriteString(open)
	if p.Value != "" {
		pw.WriteString(p.Value)
	}
	pw.WriteString(close)
}

// writeResponse writes a <D:response> for one resource.
func writeResponse(pw *propWriter, href string, props []propOut, namesOnly bool) {
	pw.WriteString("<D:response><D:href>" + escapeXML(href) + "</D:href>")
	if namesOnly {
		pw.WriteString("<D:propstat><D:prop>")
		for _, p := range props {
			open, close := propTagOpenClose(p.Name)
			pw.WriteString(open + close)
		}
		pw.WriteString("</D:prop><D:status>HTTP/1.1 200 OK</D:status></D:propstat></D:response>")
		return
	}

	var okProps, missing []propOut
	for _, p := range props {
		if p.Status == http.StatusOK {
			okProps = append(okProps, p)
		} else {
			missing = append(missing, p)
		}
	}
	if len(okProps) > 0 || len(missing) == 0 {
		pw.WriteString("<D:propstat><D:prop>")
		for _, p := range okProps {
			writePropOut(pw, p)
		}
		pw.WriteString("</D:prop><D:status>HTTP/1.1 200 OK</D:status></D:propstat>")
	}
	if len(missing) > 0 {
		pw.WriteString("<D:propstat><D:prop>")
		for _, p := range missing {
			open, close := propTagOpenClose(p.Name)
			pw.WriteString(open + close)
		}
		pw.WriteString("</D:prop><D:status>HTTP/1.1 404 Not Found</D:status></D:propstat>")
	}
	pw.WriteString("</D:response>")
}

// readBody reads a request body with a sane upper bound.
func readBody(r *http.Request) ([]byte, error) {
	return io.ReadAll(io.LimitReader(r.Body, 1<<20))
}
