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
	_ "image/gif"  // DecodeConfig for GIF logos
	_ "image/jpeg" // DecodeConfig for JPEG logos
	_ "image/png"  // DecodeConfig for PNG logos
	"io"
	"net/http"
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
	"feMerge": true, "feMergeNode": true, "feColorMatrix": true, "feBlend": true,
	"metadata": true,
}

var errSVG = &ValidationError{Message: "an SVG logo can only contain shapes, text, gradients and filters: no scripts, styles, embedded images, event handlers or links outside the file"}

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
		case xml.EndElement:
			depth--
		case xml.Directive:
			return errSVG // <!DOCTYPE ...> and entity declarations
		case xml.ProcInst:
			if t.Target != "xml" {
				return errSVG // e.g. <?xml-stylesheet ...?>
			}
		case xml.CharData:
			if depth == 0 && len(bytes.TrimSpace(t)) > 0 {
				return errBadLogo // text outside the root
			}
		}
	}
	if root != "svg" {
		return errBadLogo
	}
	return nil
}

// safeSVGAttr allows an attribute unless it is an event handler, a style
// attribute (CSS can import or fetch), or a link or url() that leaves the
// file.
func safeSVGAttr(a xml.Attr) bool {
	name := strings.ToLower(a.Name.Local)
	v := strings.ToLower(strings.TrimSpace(a.Value))
	switch {
	case strings.HasPrefix(name, "on"):
		return false
	case name == "style":
		return false
	case name == "href" || name == "src":
		return strings.HasPrefix(v, "#")
	}
	// Presentation attributes may use url(#id) for gradients, clips and
	// filters, and nothing else.
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

// SetFormatLogo stores a format's logo and returns the format, whose
// logo_url now points at the new version.
func (s *TrackingService) SetFormatLogo(ctx context.Context, id int64, data []byte) (repository.WatchFormat, error) {
	ct, err := logoType(data)
	if err != nil {
		return repository.WatchFormat{}, err
	}
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
