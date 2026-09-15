package caldav

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	stdpath "path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/emersion/go-ical"
	emcaldav "github.com/emersion/go-webdav/caldav"
)

// HTTP layer for RFC 6578 sync-collection support, which go-webdav's server
// does not implement. Two behaviors are added in front of the go-webdav
// handler (both live inside auth):
//
//   - REPORT sync-collection on a calendar collection is served directly from
//     Backend.SyncCollection: initial sync returns every member, later syncs
//     return only changed/removed members plus a new opaque sync token.
//   - PROPFIND responses that ask for DAV:sync-token on a collection get the
//     current token injected into the collection's 200 propstat, so clients
//     can also discover the token via the classic property route.
//
// All other requests pass through untouched.

const (
	davNS    = "DAV:"
	caldavNS = "urn:ietf:params:xml:ns:caldav"

	xmlContentType = `application/xml; charset="utf-8"`
)

// NewSyncMiddleware wraps a go-webdav CalDAV handler with sync-collection
// support.
func NewSyncMiddleware(b *Backend, next http.Handler) http.Handler {
	return &syncMiddleware{b: b, next: next}
}

type syncMiddleware struct {
	b    *Backend
	next http.Handler
}

func (m *syncMiddleware) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "REPORT":
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "caldav: cannot read request body", http.StatusBadRequest)
			return
		}
		if isSyncCollectionRequest(body) && isSyncablePath(r.URL.Path) {
			m.handleSyncCollection(w, r, body)
			return
		}
		forwardRequest(m.next, w, r, body)
	case "PROPFIND":
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "caldav: cannot read request body", http.StatusBadRequest)
			return
		}
		if isSyncablePath(r.URL.Path) && propfindRequestsSyncToken(body) {
			m.handlePropfindWithSyncToken(w, r, body)
			return
		}
		forwardRequest(m.next, w, r, body)
	default:
		m.next.ServeHTTP(w, r)
	}
}

// forwardRequest re-attaches an already-consumed body and lets the inner
// handler process the request.
func forwardRequest(next http.Handler, w http.ResponseWriter, r *http.Request, body []byte) {
	r.Body = io.NopCloser(bytes.NewReader(body))
	r.ContentLength = int64(len(body))
	next.ServeHTTP(w, r)
}

// isSyncablePath reports whether p is one of the two fixed collections.
func isSyncablePath(p string) bool {
	switch stdpath.Clean(p) {
	case stdpath.Clean(CalendarPath), stdpath.Clean(TasksPath):
		return true
	}
	return false
}

// isSyncCollectionRequest reports whether the REPORT body carries a
// DAV:sync-collection root element (prefix-independent).
func isSyncCollectionRequest(body []byte) bool {
	name, ok := xmlRootName(body)
	return ok && name.Space == davNS && name.Local == "sync-collection"
}

// propfindRequestsSyncToken reports whether a PROPFIND body asks for the
// DAV:sync-token property anywhere in its prop list.
func propfindRequestsSyncToken(body []byte) bool {
	if len(body) == 0 {
		return false
	}
	var req struct {
		XMLName xml.Name
		Prop    struct {
			Props []xml.Name `xml:",any"`
		} `xml:"prop"`
	}
	if err := xml.Unmarshal(body, &req); err != nil {
		return false
	}
	for _, p := range req.Prop.Props {
		if (p.Space == davNS || p.Space == "") && p.Local == "sync-token" {
			return true
		}
	}
	return false
}

// xmlRootName returns the document element's name.
func xmlRootName(body []byte) (xml.Name, bool) {
	dec := xml.NewDecoder(bytes.NewReader(body))
	for {
		tok, err := dec.Token()
		if err != nil {
			return xml.Name{}, false
		}
		if se, ok := tok.(xml.StartElement); ok {
			return se.Name, true
		}
	}
}

// ---------------------------------------------------------------------------
// REPORT sync-collection
// ---------------------------------------------------------------------------

type syncRequestBody struct {
	XMLName   xml.Name `xml:"DAV: sync-collection"`
	SyncToken string   `xml:"sync-token"`
	SyncLevel string   `xml:"sync-level"`
	Prop      struct {
		Props []*xmlNode `xml:",any"`
	} `xml:"prop"`
}

func (m *syncMiddleware) handleSyncCollection(w http.ResponseWriter, r *http.Request, body []byte) {
	var req syncRequestBody
	if err := xml.Unmarshal(body, &req); err != nil {
		http.Error(w, "caldav: malformed sync-collection request", http.StatusBadRequest)
		return
	}

	result, err := m.b.SyncCollection(r.Context(), r.URL.Path, req.SyncToken)
	if err != nil {
		if errors.Is(err, ErrInvalidSyncToken) {
			// RFC 6578 §3.8: tell the client its token is stale. It retries
			// without a token for a full resync.
			w.Header().Set("Content-Type", xmlContentType)
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>` +
				`<D:error xmlns:D="DAV:"><D:valid-sync-token/></D:error>`))
			return
		}
		writeBackendError(w, err)
		return
	}

	props := requestedPropNames(req.Prop.Props)
	out, err := encodeSyncResponse(result, props)
	if err != nil {
		http.Error(w, "caldav: failed to encode sync response", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", xmlContentType)
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusMultiStatus)
	_, _ = w.Write(out)
}

// requestedPropNames turns the request prop list into a lookup. An empty
// request defaults to DAV:getetag, matching what clients use for the
// etag-discovery phase.
func requestedPropNames(nodes []*xmlNode) map[xml.Name]bool {
	props := make(map[xml.Name]bool, len(nodes))
	for _, n := range nodes {
		props[n.Start.Name] = true
	}
	if len(props) == 0 {
		props[xml.Name{Space: davNS, Local: "getetag"}] = true
	}
	return props
}

// supportedSyncProp is the property set this server can render inside a sync
// report. Anything else is reported back with a 404 propstat.
var supportedSyncProps = map[xml.Name]bool{
	{Space: davNS, Local: "getetag"}:          true,
	{Space: davNS, Local: "getcontentlength"}: true,
	{Space: davNS, Local: "getcontenttype"}:   true,
	{Space: davNS, Local: "getlastmodified"}:  true,
	{Space: caldavNS, Local: "calendar-data"}: true,
}

func encodeSyncResponse(result *SyncResult, props map[xml.Name]bool) ([]byte, error) {
	var b bytes.Buffer
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	b.WriteString(`<D:multistatus xmlns:D="DAV:" xmlns:C="urn:ietf:params:xml:ns:caldav">`)

	unknown := make([]xml.Name, 0)
	for name := range props {
		if !supportedSyncProps[name] {
			unknown = append(unknown, name)
		}
	}
	sortPropNames(unknown)

	for _, entry := range result.Entries {
		switch {
		case entry.RemovedHref != "":
			writeElemStart(&b, "D:response")
			writeElem(&b, "D:href", entry.RemovedHref)
			writeElem(&b, "D:status", "HTTP/1.1 404 Not Found")
			writeElemEnd(&b, "D:response")
		case entry.Object != nil:
			if err := encodeChangedResponse(&b, entry.Object, props, unknown); err != nil {
				return nil, err
			}
		}
	}

	// New token: the checkpoint the client presents on its next sync.
	writeElem(&b, "D:sync-token", result.NewToken)
	b.WriteString(`</D:multistatus>`)
	return b.Bytes(), nil
}

func encodeChangedResponse(b *bytes.Buffer, co *emcaldav.CalendarObject, props map[xml.Name]bool, unknown []xml.Name) error {
	writeElemStart(b, "D:response")
	writeElem(b, "D:href", co.Path)

	// 200 propstat with every supported property the client requested.
	writeElemStart(b, "D:propstat")
	writeElemStart(b, "D:prop")
	if props[xml.Name{Space: davNS, Local: "getetag"}] {
		// Mirror go-webdav's internal.ETag.String() quoting byte-for-byte so
		// etags from PROPFIND/multiget and from sync-collection compare equal
		// on the client (our stored ETag is itself already quoted).
		writeElem(b, "D:getetag", strconv.Quote(co.ETag))
	}
	if props[xml.Name{Space: davNS, Local: "getcontentlength"}] {
		writeElem(b, "D:getcontentlength", strconv.FormatInt(co.ContentLength, 10))
	}
	if props[xml.Name{Space: davNS, Local: "getcontenttype"}] {
		writeElem(b, "D:getcontenttype", ical.MIMEType)
	}
	if props[xml.Name{Space: davNS, Local: "getlastmodified"}] && !co.ModTime.IsZero() {
		writeElem(b, "D:getlastmodified", co.ModTime.UTC().Format(time.RFC1123))
	}
	if props[xml.Name{Space: caldavNS, Local: "calendar-data"}] {
		var data bytes.Buffer
		if err := ical.NewEncoder(&data).Encode(co.Data); err != nil {
			return err
		}
		writeElem(b, "C:calendar-data", data.String())
	}
	writeElemEnd(b, "D:prop")
	writeElem(b, "D:status", "HTTP/1.1 200 OK")
	writeElemEnd(b, "D:propstat")

	// Unrecognized properties come back as empty elements in a 404 propstat
	// (RFC 2518/4791 semantics) instead of failing the whole sync.
	if len(unknown) > 0 {
		writeElemStart(b, "D:propstat")
		writeElemStart(b, "D:prop")
		for _, name := range unknown {
			writeEmptyProp(b, name)
		}
		writeElemEnd(b, "D:prop")
		writeElem(b, "D:status", "HTTP/1.1 404 Not Found")
		writeElemEnd(b, "D:propstat")
	}

	writeElemEnd(b, "D:response")
	return nil
}

// writeElemStart writes <tag>.
func writeElemStart(b *bytes.Buffer, tag string) {
	b.WriteByte('<')
	b.WriteString(tag)
	b.WriteByte('>')
}

// writeElemEnd writes </tag>.
func writeElemEnd(b *bytes.Buffer, tag string) {
	b.WriteString("</")
	b.WriteString(tag)
	b.WriteByte('>')
}

// writeElem writes <tag>escaped-text</tag>.
func writeElem(b *bytes.Buffer, tag, text string) {
	writeElemStart(b, tag)
	xml.EscapeText(b, []byte(text))
	writeElemEnd(b, tag)
}

var ncNameRe = regexp.MustCompile(`^[A-Za-z_][\w.\-]*$`)

// writeEmptyProp renders an empty property element for an unknown requested
// prop, declaring an inline namespace when it is neither DAV nor caldav.
func writeEmptyProp(b *bytes.Buffer, name xml.Name) {
	if !ncNameRe.MatchString(name.Local) {
		return // malformed client prop name; omit rather than emit bad XML
	}
	switch name.Space {
	case davNS, "":
		b.WriteString("<D:")
		b.WriteString(name.Local)
		b.WriteString("/>")
	case caldavNS:
		b.WriteString("<C:")
		b.WriteString(name.Local)
		b.WriteString("/>")
	default:
		b.WriteString(`<X:`)
		b.WriteString(name.Local)
		b.WriteString(` xmlns:X="`)
		xml.EscapeText(b, []byte(name.Space))
		b.WriteString(`"/>`)
	}
}

// sortPropNames orders requested-but-unknown property names for stable output.
func sortPropNames(names []xml.Name) {
	sort.Slice(names, func(i, j int) bool {
		if names[i].Space != names[j].Space {
			return names[i].Space < names[j].Space
		}
		return names[i].Local < names[j].Local
	})
}

// stripNamespaceDecls removes xmlns / xmlns:prefix attributes produced by the
// XML decoder so re-encoding does not duplicate namespace declarations.
func stripNamespaceDecls(attrs []xml.Attr) []xml.Attr {
	out := attrs[:0]
	for _, a := range attrs {
		if a.Name.Space == "http://www.w3.org/2000/xmlns/" || a.Name.Local == "xmlns" {
			continue
		}
		out = append(out, a)
	}
	return out
}

// writeBackendError maps backend HTTP errors (webdav.HTTPError carries a
// status prefix in its Error() string) to a status code.
func writeBackendError(w http.ResponseWriter, err error) {
	switch {
	case strings.HasPrefix(err.Error(), "404"):
		http.Error(w, err.Error(), http.StatusNotFound)
	case strings.HasPrefix(err.Error(), "403"):
		http.Error(w, err.Error(), http.StatusForbidden)
	case strings.HasPrefix(err.Error(), "400"):
		http.Error(w, err.Error(), http.StatusBadRequest)
	default:
		http.Error(w, "caldav: backend error", http.StatusServiceUnavailable)
	}
}

// ---------------------------------------------------------------------------
// PROPFIND sync-token injection
// ---------------------------------------------------------------------------

func (m *syncMiddleware) handlePropfindWithSyncToken(w http.ResponseWriter, r *http.Request, body []byte) {
	rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
	forwardRequest(m.next, rec, r, body)

	// Only rewrite successful multistatus responses for the collection.
	if rec.status != http.StatusMultiStatus || !bytes.Contains(rec.buf.Bytes(), []byte("<multistatus")) {
		flushRecorder(w, rec)
		return
	}

	token, err := m.b.CurrentSyncToken(r.Context())
	if err != nil {
		flushRecorder(w, rec)
		return
	}
	rewritten, err := injectSyncToken(rec.buf.Bytes(), stdpath.Clean(r.URL.Path), token)
	if err != nil {
		// Never break ordinary PROPFIND over an injection failure.
		flushRecorder(w, rec)
		return
	}

	// Headers were already set on the outer response (rec.Header() delegates
	// to w.Header()); only drop length/encoding computed for the old body and
	// let outer compression middleware recompute them.
	h := w.Header()
	h.Del("Content-Length")
	h.Del("Content-Encoding")
	w.WriteHeader(rec.status)
	_, _ = w.Write(rewritten)
}

// flushRecorder forwards a buffered response unchanged.
func flushRecorder(w http.ResponseWriter, rec *statusRecorder) {
	w.Header().Del("Content-Length")
	w.Header().Del("Content-Encoding")
	w.WriteHeader(rec.status)
	_, _ = w.Write(rec.buf.Bytes())
}

type statusRecorder struct {
	http.ResponseWriter
	status int
	buf    bytes.Buffer
	wrote  bool
}

func (r *statusRecorder) WriteHeader(code int) {
	if r.wrote {
		return
	}
	r.status = code
	r.wrote = true
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if !r.wrote {
		r.WriteHeader(http.StatusOK)
	}
	return r.buf.Write(b)
}

// multistatus XML model for PROPFIND response rewriting. Arbitrary property
// elements round-trip through xmlNode.
type multistatusXML struct {
	XMLName   xml.Name       `xml:"DAV: multistatus"`
	Responses []*responseXML `xml:"response"`
}

type responseXML struct {
	XMLName   xml.Name     `xml:"DAV: response"`
	Href      string       `xml:"href"`
	PropStats []propstatXML `xml:"propstat"`
	Status    string       `xml:"status,omitempty"`
}

type propstatXML struct {
	XMLName xml.Name `xml:"DAV: propstat"`
	Prop    propXML  `xml:"prop"`
	Status  string   `xml:"status"`
}

type propXML struct {
	XMLName  xml.Name  `xml:"DAV: prop"`
	Children []*xmlNode `xml:",any"`
}

// xmlNode is a generic round-trippable XML element (element children and
// text interleaved) so PROPFIND bodies we don't model survive a parse/encode
// cycle unchanged in meaning.
type xmlNode struct {
	Start   xml.StartElement
	Content []xmlNodeContent
}

type xmlNodeContent struct {
	Node *xmlNode
	Text string
}

func (n *xmlNode) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	// Drop xmlns declarations: encoding/xml re-emits them from Name.Space on
	// marshal, so keeping the originals produces duplicate (and spec-illegal)
	// attributes like `<x xmlns="DAV:" xmlns="DAV:">`.
	start.Attr = stripNamespaceDecls(start.Attr)
	n.Start = start
	for {
		tok, err := d.Token()
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			child := &xmlNode{}
			if err := child.UnmarshalXML(d, t); err != nil {
				return err
			}
			n.Content = append(n.Content, xmlNodeContent{Node: child})
		case xml.CharData:
			n.Content = append(n.Content, xmlNodeContent{Text: string(t)})
		case xml.EndElement:
			return nil
		}
	}
}

func (n *xmlNode) MarshalXML(e *xml.Encoder, _ xml.StartElement) error {
	if err := e.EncodeToken(n.Start); err != nil {
		return err
	}
	for _, c := range n.Content {
		if c.Node != nil {
			if err := c.Node.MarshalXML(e, xml.StartElement{}); err != nil {
				return err
			}
		} else if err := e.EncodeToken(xml.CharData([]byte(c.Text))); err != nil {
			return err
		}
	}
	return e.EncodeToken(n.Start.End())
}

// injectSyncToken parses a PROPFIND multistatus, fills DAV:sync-token on the
// collection response's 200 propstat and drops the 404 propstat the
// go-webdav backend emits for the unimplemented property. The collection is
// matched by href using both slashed and unslashed forms.
func injectSyncToken(body []byte, cleanPath, token string) ([]byte, error) {
	var ms multistatusXML
	if err := xml.Unmarshal(body, &ms); err != nil {
		return nil, err
	}

	matched := false
	for _, resp := range ms.Responses {
		if !hrefMatchesCollection(resp.Href, cleanPath) {
			continue
		}
		matched = true

		// Drop every propstat that carries sync-token (some go-webdav
		// responses include a 404 propstat for the unimplemented property);
		// collect a surviving 200 propstat so the token can join its prop.
		propstats := resp.PropStats[:0]
		ok200 := -1
		for _, ps := range resp.PropStats {
			if propstatHasSyncToken(&ps) {
				continue
			}
			if strings.Contains(ps.Status, "200") && ok200 < 0 {
				ok200 = len(propstats)
			}
			propstats = append(propstats, ps)
		}
		resp.PropStats = propstats

		tokenNode := &xmlNode{
			Start:   xml.StartElement{Name: xml.Name{Space: davNS, Local: "sync-token"}},
			Content: []xmlNodeContent{{Text: token}},
		}
		if ok200 >= 0 {
			resp.PropStats[ok200].Prop.Children = append(
				resp.PropStats[ok200].Prop.Children, tokenNode)
		} else {
			// go-webdav may emit no propstat at all when sync-token was the
			// only requested property; synthesize a 200 propstat for it.
			resp.PropStats = append(resp.PropStats, propstatXML{
				Prop:   propXML{Children: []*xmlNode{tokenNode}},
				Status: "HTTP/1.1 200 OK",
			})
		}
		break
	}
	if !matched {
		return nil, fmt.Errorf("caldav: collection response not found in PROPFIND body")
	}

	out, err := xml.MarshalIndent(&ms, "", "  ")
	if err != nil {
		return nil, err
	}
	return append([]byte(xml.Header), out...), nil
}

// hrefMatchesCollection compares an href against the cleaned request path;
// percent-decoding is unnecessary because our paths and ids are ASCII-safe.
func hrefMatchesCollection(href, cleanPath string) bool {
	h := strings.TrimSpace(href)
	return h == cleanPath || stdpath.Clean(h) == cleanPath || strings.TrimRight(h, "/") == cleanPath
}

func propstatHasSyncToken(ps *propstatXML) bool {
	for _, c := range ps.Prop.Children {
		if (c.Start.Name.Space == davNS || c.Start.Name.Space == "") && c.Start.Name.Local == "sync-token" {
			return true
		}
	}
	return false
}
