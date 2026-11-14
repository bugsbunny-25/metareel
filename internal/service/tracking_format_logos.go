package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"image"
	"image/gif"
	_ "image/jpeg" // DecodeConfig for JPEG logos
	"image/png"
	"io"
	"net/http"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/bugsbunny-25/metareel/internal/repository"
)

// MaxFormatLogoBytes caps an uploaded logo; logos are small marks, not photos.
const MaxFormatLogoBytes = 512 << 10

const (
	// maxLogoSide and maxLogoPixels stop a small file that decodes to a huge
	// bitmap (a decompression bomb) in every viewer's browser.
	maxLogoSide   = 4096
	maxLogoPixels = 8 << 20
	// maxSVGElements bounds the work a browser does to draw an SVG logo.
	maxSVGElements = 2000
)

var errBadLogo = &ValidationError{Message: "the logo must be a PNG, JPEG, WebP, GIF or SVG image"}

// logoType works out what an upload is from its bytes, never from what the
// client claims, and checks it is safe to show everyone: a raster image of
// sane dimensions, or an SVG made only of drawing elements (see checkSVG).
//
// This is one layer. The logo route also serves every logo with a sandboxing
// CSP and nosniff, and the apps only ever show logos through <img>, where
// browsers run no script and load nothing external.
func logoType(data []byte) (string, error) {
	switch {
	case len(data) == 0:
		return "", &ValidationError{Message: "the logo is empty"}
	case len(data) > MaxFormatLogoBytes:
		return "", &ValidationError{Message: "the logo must be at most 512 KB"}
	}
	switch ct := http.DetectContentType(data); ct {
	case "image/png", "image/jpeg", "image/gif":
		cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
		if err != nil {
			return "", errBadLogo
		}
		return ct, checkSize(cfg.Width, cfg.Height)
	case "image/webp":
		w, h, err := webpSize(data)
		if err != nil {
			return "", errBadLogo
		}
		return ct, checkSize(w, h)
	}
	if err := checkSVG(data); err != nil {
		return "", err
	}
	return "image/svg+xml", nil
}

func checkSize(w, h int) error {
	if w <= 0 || h <= 0 {
		return errBadLogo
	}
	if w > maxLogoSide || h > maxLogoSide || w*h > maxLogoPixels {
		return &ValidationError{Message: "the logo is too large in pixels; keep it within 4096 on each side"}
	}
	return nil
}

// webpSize reads a WebP's canvas size from its header (lossy VP8, lossless
// VP8L or extended VP8X), without a WebP decoder.
func webpSize(b []byte) (int, int, error) {
	if len(b) < 30 || string(b[0:4]) != "RIFF" || string(b[8:12]) != "WEBP" {
		return 0, 0, errors.New("not a webp")
	}
	switch string(b[12:16]) {
	case "VP8X":
		w := 1 + int(uint32(b[24])|uint32(b[25])<<8|uint32(b[26])<<16)
		h := 1 + int(uint32(b[27])|uint32(b[28])<<8|uint32(b[29])<<16)
		return w, h, nil
	case "VP8 ":
		// Frame header: 3-byte tag, start code 9d 01 2a, then 14-bit sizes.
		if b[23] != 0x9d || b[24] != 0x01 || b[25] != 0x2a {
			return 0, 0, errors.New("bad vp8 frame")
		}
		return int(binary.LittleEndian.Uint16(b[26:28]) & 0x3fff), int(binary.LittleEndian.Uint16(b[28:30]) & 0x3fff), nil
	case "VP8L":
		if b[20] != 0x2f {
			return 0, 0, errors.New("bad vp8l signature")
		}
		bits := binary.LittleEndian.Uint32(b[21:25])
		return int(bits&0x3fff) + 1, int((bits>>14)&0x3fff) + 1, nil
	}
	return 0, 0, errors.New("unknown webp chunk")
}

// svgElements are the drawing elements a logo can use. Anything else
// (script, style, foreignObject, image, a, animate, iframe, ...) is refused,
// matched on the local name so a namespace prefix doesn't hide it.
var svgElements = map[string]bool{
	"svg": true, "g": true, "defs": true, "symbol": true, "use": true, "title": true, "desc": true,
	"path": true, "rect": true, "circle": true, "ellipse": true, "line": true, "polyline": true, "polygon": true,
	"text": true, "tspan": true, "textPath": true,
	"linearGradient": true, "radialGradient": true, "stop": true, "pattern": true,
	"clipPath": true, "mask": true, "marker": true,
	"filter": true, "feGaussianBlur": true, "feOffset": true, "feFlood": true, "feComposite": true,
	"feMerge": true, "feMergeNode": true, "feColorMatrix": true, "feBlend": true, "feDropShadow": true,
	"feComponentTransfer": true, "feFuncR": true, "feFuncG": true, "feFuncB": true, "feFuncA": true,
	"feMorphology": true,
	"metadata":     true, "style": true, "switch": true,
}

// editorNamespaces are the namespaces design tools add for their own
// bookkeeping (Inkscape's named views, RDF licence blocks). Browsers don't
// render them, so their elements and everything inside are skipped.
var editorNamespaces = map[string]bool{
	"http://www.inkscape.org/namespaces/inkscape":        true,
	"http://sodipodi.sourceforge.net/DTD/sodipodi-0.dtd": true,
	"http://www.w3.org/1999/02/22-rdf-syntax-ns#":        true,
	"http://creativecommons.org/ns#":                     true,
	"http://purl.org/dc/elements/1.1/":                   true,
	// Illustrator's "preserve editing" data and its private extensions.
	"http://ns.adobe.com/AdobeIllustrator/10.0/":        true,
	"http://ns.adobe.com/Variables/1.0/":                true,
	"http://ns.adobe.com/ImageReplacement/1.0/":         true,
	"http://ns.adobe.com/SaveForWeb/1.0/":               true,
	"http://ns.adobe.com/Extensibility/1.0/":            true,
	"http://ns.adobe.com/Flows/1.0/":                    true,
	"http://ns.adobe.com/GenericCustomNamespace/1.0/":   true,
	"http://ns.adobe.com/XPath/1.0/":                    true,
	"http://ns.adobe.com/AdobeSVGViewerExtensions/3.0/": true,
}

var errSVG = &ValidationError{Message: "an SVG logo can only contain shapes, text, gradients, filters and plain CSS: no scripts, embedded images, event handlers, CSS @-rules or links outside the file"}

// checkSVG parses an SVG and accepts it only if it is UTF-8 XML whose root is
// <svg>, every element is a drawing element, no attribute is an event handler,
// every link (href, url()) points inside the file, and there is no DOCTYPE
// (entities) or other directive. Attribute values are checked after the
// parser has decoded character references, so "&#106;avascript:" is seen as
// what it is.
func checkSVG(data []byte) error {
	if !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0 {
		return errBadLogo // UTF-16 and other encodings can hide markup from any check
	}
	head := data
	if len(head) > 4096 {
		head = head[:4096]
	}
	if !bytes.Contains(bytes.ToLower(head), []byte("<svg")) {
		return errBadLogo
	}
	d := xml.NewDecoder(bytes.NewReader(data))
	d.Strict = true
	d.CharsetReader = func(label string, r io.Reader) (io.Reader, error) {
		if strings.EqualFold(label, "utf-8") || strings.EqualFold(label, "utf8") {
			return r, nil
		}
		return nil, errors.New("only UTF-8")
	}
	depth, elements, root := 0, 0, ""
	skip := 0 // >0 while inside metadata or an editor element
	inStyle := false
	for {
		tok, err := d.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return errBadLogo
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if skip > 0 || editorNamespaces[t.Name.Space] || (depth > 0 && (t.Name.Local == "metadata" || t.Name.Local == "foreignObject")) {
				// Editor bookkeeping, metadata and Illustrator's foreignObject
				// (pointing at its private editing data) aren't drawn, so the
				// editor elements' names don't matter. But an SVG or XHTML
				// element hidden inside one (a <script>, an <iframe>, a <div>)
				// still counts: those must be plain drawing elements, a <style>
				// isn't allowed in there (its CSS isn't checked), and every
				// attribute is still checked.
				editor := editorNamespaces[t.Name.Space]
				wrapper := skip == 0 && (t.Name.Local == "metadata" || t.Name.Local == "foreignObject")
				if !editor && !wrapper && (!svgElements[t.Name.Local] || t.Name.Local == "style") {
					return errSVG
				}
				for _, a := range t.Attr {
					if !safeSVGAttr(a) {
						return errSVG
					}
				}
				skip++
				continue
			}
			elements++
			if elements > maxSVGElements {
				return &ValidationError{Message: "the SVG logo is too complex; simplify it or export a PNG"}
			}
			if depth == 0 {
				if root != "" {
					return errBadLogo // a second root element
				}
				root = t.Name.Local
			}
			depth++
			if !svgElements[t.Name.Local] {
				return errSVG
			}
			for _, a := range t.Attr {
				if !safeSVGAttr(a) {
					return errSVG
				}
			}
			inStyle = t.Name.Local == "style"
		case xml.EndElement:
			if skip > 0 {
				skip--
				continue
			}
			depth--
			inStyle = false
		case xml.Directive:
			// A DOCTYPE is fine. Its internal subset may only declare plain
			// entities with URL-like values: Illustrator writes the SVG and
			// XLink namespaces that way (<!ENTITY ns_svg "http://...">). They
			// are handed to the parser, so whatever uses them is checked
			// expanded. Anything else (markup, parameter entities, element
			// declarations) and any other directive is refused.
			dir := strings.TrimSpace(string(t))
			if !strings.HasPrefix(dir, "DOCTYPE") {
				return errSVG
			}
			ents, ok := plainEntities(dir)
			if !ok {
				return errSVG
			}
			d.Entity = ents
		case xml.ProcInst:
			if t.Target != "xml" {
				return errSVG // e.g. <?xml-stylesheet ...?>
			}
		case xml.CharData:
			if skip > 0 {
				continue
			}
			if depth == 0 && len(bytes.TrimSpace(t)) > 0 {
				return errBadLogo // text outside the root
			}
			if inStyle && !safeCSS(string(t)) {
				return errSVG
			}
		}
	}
	if root != "svg" {
		return errBadLogo
	}
	return nil
}

var (
	entityDecl = regexp.MustCompile(`<!ENTITY\s+([A-Za-z_][A-Za-z0-9_.-]*)\s+"([^"]*)"\s*>`)
	urlLike    = regexp.MustCompile(`^[A-Za-z0-9:/._#?=+~-]*$`)
)

// plainEntities reads a DOCTYPE's internal subset, if any, accepting only
// up to 20 <!ENTITY name "url-like value"> declarations.
func plainEntities(doctype string) (map[string]string, bool) {
	open := strings.IndexByte(doctype, '[')
	if open < 0 {
		return nil, !strings.Contains(doctype, "]")
	}
	end := strings.LastIndexByte(doctype, ']')
	if end < open {
		return nil, false
	}
	ents := map[string]string{}
	rest := entityDecl.ReplaceAllStringFunc(doctype[open+1:end], func(decl string) string {
		m := entityDecl.FindStringSubmatch(decl)
		if !urlLike.MatchString(m[2]) || len(ents) >= 20 {
			return decl // left in rest, so the subset is refused
		}
		ents[m[1]] = m[2]
		return ""
	})
	return ents, strings.TrimSpace(rest) == ""
}

// safeSVGAttr allows an attribute unless it is an event handler, unsafe CSS,
// or a link or url() that leaves the file.
func safeSVGAttr(a xml.Attr) bool {
	name := strings.ToLower(a.Name.Local)
	v := strings.ToLower(strings.TrimSpace(a.Value))
	switch {
	case strings.HasPrefix(name, "on"):
		return false
	case name == "style":
		return safeCSS(v)
	case name == "href" || name == "src":
		return strings.HasPrefix(v, "#")
	}
	return localURLsOnly(v)
}

// safeCSS allows plain declarations and class rules (what design tools
// export) but no @-rules (@import, @font-face), no escapes that could spell
// anything, no legacy script hooks, and url() only to #ids.
func safeCSS(css string) bool {
	c := strings.ToLower(css)
	for _, bad := range []string{"@", "\\", "expression", "javascript", "behavior", "binding", "</"} {
		if strings.Contains(c, bad) {
			return false
		}
	}
	return localURLsOnly(c)
}

// localURLsOnly is false when a value has a url() that isn't url(#id), the
// only kind gradients, clips and filters need.
func localURLsOnly(v string) bool {
	for rest := v; ; {
		i := strings.Index(rest, "url(")
		if i < 0 {
			break
		}
		ref := strings.TrimLeft(rest[i+4:], " \t\n'\"")
		if !strings.HasPrefix(ref, "#") {
			return false
		}
		rest = ref
	}
	return !strings.Contains(v, "javascript:")
}

// trimTransparent crops fully transparent edges off a PNG or single-frame
// GIF, so a logo exported on a big empty canvas isn't drawn tiny. Only the
// empty border goes; the result is a PNG. Anything else, or an image with
// less than 2% to gain, is returned untouched.
func trimTransparent(data []byte, ct string) ([]byte, string) {
	var img image.Image
	switch ct {
	case "image/png":
		m, err := png.Decode(bytes.NewReader(data))
		if err != nil {
			return data, ct
		}
		img = m
	case "image/gif":
		g, err := gif.DecodeAll(bytes.NewReader(data))
		if err != nil || len(g.Image) != 1 {
			return data, ct // animated GIFs are left alone
		}
		img = g.Image[0]
	default:
		return data, ct
	}
	b := img.Bounds()
	minX, minY, maxX, maxY := b.Max.X, b.Max.Y, b.Min.X-1, b.Min.Y-1
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if _, _, _, a := img.At(x, y).RGBA(); a > 0 {
				minX, maxX = min(minX, x), max(maxX, x)
				minY, maxY = min(minY, y), max(maxY, y)
			}
		}
	}
	crop := image.Rect(minX, minY, maxX+1, maxY+1)
	if maxX < minX || crop.Dx()*crop.Dy()*100 >= b.Dx()*b.Dy()*98 {
		return data, ct // fully transparent, or nothing worth trimming
	}
	sub, ok := img.(interface {
		SubImage(image.Rectangle) image.Image
	})
	if !ok {
		return data, ct
	}
	var buf bytes.Buffer
	if err := (&png.Encoder{CompressionLevel: png.BestCompression}).Encode(&buf, sub.SubImage(crop)); err != nil {
		return data, ct
	}
	return buf.Bytes(), "image/png"
}

// SetFormatLogo stores a format's logo and returns the format, whose
// logo_url now points at the new version.
func (s *TrackingService) SetFormatLogo(ctx context.Context, id int64, data []byte) (repository.WatchFormat, error) {
	ct, err := logoType(data)
	if err != nil {
		return repository.WatchFormat{}, err
	}
	data, ct = trimTransparent(data, ct)
	sum := sha256.Sum256(data)
	if err := s.write.SetFormatLogo(ctx, id, ct, data, hex.EncodeToString(sum[:6])); err != nil {
		return repository.WatchFormat{}, err
	}
	return s.write.Format(ctx, id)
}

// ClearFormatLogo removes a format's logo.
func (s *TrackingService) ClearFormatLogo(ctx context.Context, id int64) (repository.WatchFormat, error) {
	if err := s.write.ClearFormatLogo(ctx, id); err != nil {
		return repository.WatchFormat{}, err
	}
	return s.write.Format(ctx, id)
}

// FormatLogo returns a format's logo by slug; sql.ErrNoRows when it has none.
func (s *TrackingService) FormatLogo(ctx context.Context, slug string) (repository.FormatLogo, error) {
	return s.read.FormatLogoBySlug(ctx, strings.ToLower(strings.TrimSpace(slug)))
}
