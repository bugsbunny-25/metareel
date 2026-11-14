package service

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
	"unicode/utf16"
)

func pngOf(t *testing.T, w, h int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewGray(image.Rect(0, 0, w, h))); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// webpX is a VP8X WebP header declaring a w×h canvas (enough for sniffing and
// for webpSize; browsers would need the image data too).
func webpX(w, h int) []byte {
	b := make([]byte, 30)
	copy(b[0:], "RIFF")
	copy(b[8:], "WEBPVP8X")
	b[16] = 10
	w--
	h--
	b[24], b[25], b[26] = byte(w), byte(w>>8), byte(w>>16)
	b[27], b[28], b[29] = byte(h), byte(h>>8), byte(h>>16)
	return b
}

func utf16le(s string) []byte {
	out := []byte{0xff, 0xfe}
	for _, r := range utf16.Encode([]rune(s)) {
		out = append(out, byte(r), byte(r>>8))
	}
	return out
}

const svgOpen = `<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" viewBox="0 0 10 10">`

func svgWith(inner string) []byte { return []byte(svgOpen + inner + `</svg>`) }

// A logo's type comes from its bytes; it has to be a sane raster image or an
// SVG made only of drawing elements.
func TestLogoType(t *testing.T) {
	for name, tc := range map[string]struct {
		data []byte
		want string
	}{
		"png":               {pngOf(t, 120, 40), "image/png"},
		"webp":              {webpX(300, 80), "image/webp"},
		"plain svg":         {[]byte(`<?xml version="1.0" encoding="UTF-8"?>` + svgOpen + `<rect width="10" height="10" fill="#fff"/></svg>`), "image/svg+xml"},
		"svg text":          {svgWith(`<text x="0" y="8" font-family="Arial" fill="white">IMAX</text>`), "image/svg+xml"},
		"svg gradient refs": {svgWith(`<defs><linearGradient id="g"><stop offset="0" stop-color="#fff"/></linearGradient></defs><rect fill="url(#g)" width="10" height="10"/><use href="#g"/><use xlink:href="#g"/>`), "image/svg+xml"},
		// What design tools export.
		"illustrator export": {[]byte(`<?xml version="1.0" encoding="utf-8"?>
<!-- Generator: Adobe Illustrator 27.0.0 -->
<!DOCTYPE svg PUBLIC "-//W3C//DTD SVG 1.1//EN" "http://www.w3.org/Graphics/SVG/1.1/DTD/svg11.dtd">
<svg version="1.1" id="Layer_1" xmlns="http://www.w3.org/2000/svg" xml:space="preserve" viewBox="0 0 100 30">
<style type="text/css">.st0{fill:#FFFFFF;} .st1{fill:url(#g1);}</style>
<path class="st0" d="M0 0h10v10z"/><rect class="st1" style="opacity:0.8" width="5" height="5"/>
</svg>`), "image/svg+xml"},
		"illustrator namespace entities": {[]byte(`<?xml version="1.0" encoding="utf-8"?>
<!DOCTYPE svg PUBLIC "-//W3C//DTD SVG 1.1//EN" "http://www.w3.org/Graphics/SVG/1.1/DTD/svg11.dtd" [
	<!ENTITY ns_svg "http://www.w3.org/2000/svg">
	<!ENTITY ns_xlink "http://www.w3.org/1999/xlink">
]>
<svg version="1.1" xmlns="&ns_svg;" xmlns:xlink="&ns_xlink;" viewBox="0 0 10 10"><path fill="#0095D5" d="M0 0h10v10z"/></svg>`), "image/svg+xml"},
		"illustrator preserve editing": {[]byte(`<?xml version="1.0" encoding="utf-8"?>
<!DOCTYPE svg PUBLIC "-//W3C//DTD SVG 1.1//EN" "http://www.w3.org/Graphics/SVG/1.1/DTD/svg11.dtd" [
	<!ENTITY ns_ai "http://ns.adobe.com/AdobeIllustrator/10.0/">
	<!ENTITY ns_graphs "http://ns.adobe.com/Graphs/1.0/">
	<!ENTITY ns_svg "http://www.w3.org/2000/svg">
	<!ENTITY ns_xlink "http://www.w3.org/1999/xlink">
]>
<svg version="1.1" xmlns:i="&ns_ai;" xmlns="&ns_svg;" xmlns:xlink="&ns_xlink;" viewBox="0 0 10 10">
<style type="text/css">.st0{fill:#000;}</style>
<switch>
	<foreignObject requiredExtensions="&ns_ai;" x="0" y="0" width="1" height="1">
		<i:aipgfRef xlink:href="#adobe_illustrator_pgf"></i:aipgfRef>
	</foreignObject>
	<g i:extraneous="self"><path class="st0" d="M0 0h10v10z"/></g>
</switch>
<i:aipgf id="adobe_illustrator_pgf" i:pgfEncoding="zstandard/AIbase64" i:pgfVersion="24">eJzsvWmTHEl2IPZ9zfY/5EqgwW2B</i:aipgf>
</svg>`), "image/svg+xml"},
		"inkscape export": {[]byte(`<svg xmlns="http://www.w3.org/2000/svg" xmlns:inkscape="http://www.inkscape.org/namespaces/inkscape" xmlns:sodipodi="http://sodipodi.sourceforge.net/DTD/sodipodi-0.dtd" xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#" xmlns:cc="http://creativecommons.org/ns#" xmlns:dc="http://purl.org/dc/elements/1.1/" sodipodi:docname="logo.svg" inkscape:version="1.3">
<sodipodi:namedview id="nv" inkscape:zoom="1"><inkscape:grid id="g"/></sodipodi:namedview>
<metadata><rdf:RDF><cc:Work rdf:about=""><dc:format>image/svg+xml</dc:format></cc:Work></rdf:RDF></metadata>
<g inkscape:label="Layer 1" inkscape:groupmode="layer"><path style="fill:#ffffff;stroke:none" d="M0 0h10v10z"/></g></svg>`), "image/svg+xml"},
	} {
		if got, err := logoType(tc.data); err != nil || got != tc.want {
			t.Errorf("%s: logoType = %q, %v; want %q", name, got, err, tc.want)
		}
	}

	for name, data := range map[string][]byte{
		"empty":                nil,
		"too big":              append(pngOf(t, 2, 2), make([]byte, MaxFormatLogoBytes)...),
		"not an image":         []byte("hello, world"),
		"html":                 []byte("<!doctype html><html><body>hi</body></html>"),
		"html with svg inside": []byte(`<html><body>` + svgOpen + `</svg><script>alert(1)</script></body></html>`),
		// Decompression bombs: tiny files, huge canvases.
		"huge png":  pngOf(t, 5000, 2),
		"huge webp": webpX(30000, 30000),
		// Scripted or fetching SVGs, including the ways a deny-list misses them.
		"script":                 svgWith(`<script>alert(1)</script>`),
		"SCRIPT":                 svgWith(`<SCRIPT>alert(1)</SCRIPT>`),
		"prefixed script":        svgWith(`<s:script xmlns:s="http://www.w3.org/2000/svg">alert(1)</s:script>`),
		"prefixed iframe":        svgWith(`<h:iframe xmlns:h="http://www.w3.org/1999/xhtml" src="https://evil"/>`),
		"onload":                 []byte(`<svg xmlns="http://www.w3.org/2000/svg" onload="alert(1)"></svg>`),
		"ONLOAD":                 []byte(`<svg xmlns="http://www.w3.org/2000/svg" ONLOAD="alert(1)"></svg>`),
		"char-ref javascript":    svgWith(`<use href="&#106;avascript:alert(1)"/>`),
		"char-ref https use":     svgWith(`<use href="&#104;ttps://evil/x.svg#a"/>`),
		"remote xlink":           svgWith(`<use xlink:href="//evil/x.svg#a"/>`),
		"anchor":                 svgWith(`<a href="#x"><text>x</text></a>`),
		"animate href":           svgWith(`<animate attributeName="href" values="javascript:alert(1)"/>`),
		"image":                  svgWith(`<image href="#x"/>`),
		"foreignObject":          svgWith(`<foreignObject><div>x</div></foreignObject>`),
		"style @import":          svgWith(`<style>@import url(https://evil/x.css);</style>`),
		"style @font-face":       svgWith(`<style>@font-face{font-family:x;src:url(#a)}</style>`),
		"style remote url":       svgWith(`<style>.a{fill:url(https://evil/x)}</style>`),
		"style escape":           svgWith(`<style>.a{background:\\75rl(https://evil)}</style>`),
		"style in CDATA":         svgWith(`<style><![CDATA[@import "https://evil/x.css";]]></style>`),
		"style attribute":        svgWith(`<rect style="fill:url(https://evil/x)"/>`),
		"style attr escape":      svgWith(`<rect style="fill:\\75rl(//evil)"/>`),
		"script in metadata":     svgWith(`<metadata><script>alert(1)</script></metadata>`),
		"script in namedview":    []byte(`<svg xmlns="http://www.w3.org/2000/svg" xmlns:sodipodi="http://sodipodi.sourceforge.net/DTD/sodipodi-0.dtd"><sodipodi:namedview><script>alert(1)</script></sodipodi:namedview></svg>`),
		"onload in namedview":    []byte(`<svg xmlns="http://www.w3.org/2000/svg" xmlns:sodipodi="http://sodipodi.sourceforge.net/DTD/sodipodi-0.dtd"><sodipodi:namedview onload="alert(1)"/></svg>`),
		"xhtml in metadata":      svgWith(`<metadata><h:iframe xmlns:h="http://www.w3.org/1999/xhtml" src="https://evil"/></metadata>`),
		"xhtml script":           svgWith(`<h:script xmlns:h="http://www.w3.org/1999/xhtml">alert(1)</h:script>`),
		"remote url fill":        svgWith(`<rect fill="url(https://evil/x#g)"/>`),
		"quoted remote url":      svgWith(`<rect fill="url('//evil/x#g')"/>`),
		"nested entities":        []byte(`<!DOCTYPE svg [<!ENTITY a "x"><!ENTITY b "&a;&a;&a;">]>` + svgOpen + `<text>&b;</text></svg>`),
		"entity with markup":     []byte(`<!DOCTYPE svg [<!ENTITY x "<script>alert(1)</script>">]>` + svgOpen + `<g>&x;</g></svg>`),
		"entity with script url": []byte(`<!DOCTYPE svg [<!ENTITY x "javascript:alert(1)">]>` + svgOpen + `<use href="&x;"/></svg>`),
		"parameter entity":       []byte(`<!DOCTYPE svg [<!ENTITY % p "x">]>` + svgOpen + `</svg>`),
		"element declaration":    []byte(`<!DOCTYPE svg [<!ELEMENT svg ANY>]>` + svgOpen + `</svg>`),
		"external entity":        []byte(`<!DOCTYPE svg [<!ENTITY x SYSTEM "https://evil/x">]>` + svgOpen + `</svg>`),
		"foreignObject style":    svgWith(`<foreignObject><style>@import url(https://evil/x.css);</style></foreignObject>`),
		"metadata style":         svgWith(`<metadata><style>@import url(https://evil/x.css);</style></metadata>`),
		"foreignObject xhtml":    svgWith(`<foreignObject><h:div xmlns:h="http://www.w3.org/1999/xhtml"><h:img src="x" onerror="alert(1)"/></h:div></foreignObject>`),
		"stylesheet PI":          []byte(`<?xml-stylesheet href="https://evil/x.css"?>` + svgOpen + `</svg>`),
		"latin-1 encoding":       []byte(`<?xml version="1.0" encoding="ISO-8859-1"?>` + svgOpen + `</svg>`),
		"utf-16":                 utf16le(svgOpen + `<script>alert(1)</script></svg>`),
		"two roots":              []byte(svgOpen + `</svg>` + svgOpen + `</svg>`),
		"not well formed":        []byte(svgOpen + `<rect>`),
		"too complex":            svgWith(string(bytes.Repeat([]byte(`<g/>`), maxSVGElements+1))),
	} {
		got, err := logoType(data)
		if err == nil {
			t.Errorf("%s: accepted as %q, want a validation error", name, got)
		} else if !isValidation(err) {
			t.Errorf("%s: err = %v, want a validation error", name, err)
		}
	}
}

func TestWebPSize(t *testing.T) {
	if w, h, err := webpSize(webpX(640, 360)); err != nil || w != 640 || h != 360 {
		t.Errorf("VP8X = %dx%d, %v; want 640x360", w, h, err)
	}
	lossless := make([]byte, 30)
	copy(lossless, "RIFF")
	copy(lossless[8:], "WEBPVP8L")
	lossless[20] = 0x2f
	bits := uint32(99) | uint32(49)<<14 // 100 x 50
	lossless[21], lossless[22], lossless[23], lossless[24] = byte(bits), byte(bits>>8), byte(bits>>16), byte(bits>>24)
	if w, h, err := webpSize(lossless); err != nil || w != 100 || h != 50 {
		t.Errorf("VP8L = %dx%d, %v; want 100x50", w, h, err)
	}
	if _, _, err := webpSize([]byte("RIFF")); err == nil {
		t.Error("a truncated header was accepted")
	}
}

// A logo on a big transparent canvas is cropped to what's drawn; one that
// already fills its canvas is stored as uploaded.
func TestTrimTransparent(t *testing.T) {
	canvas := image.NewNRGBA(image.Rect(0, 0, 200, 100))
	for y := 30; y < 50; y++ {
		for x := 60; x < 140; x++ {
			canvas.Set(x, y, color.NRGBA{R: 255, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, canvas); err != nil {
		t.Fatal(err)
	}
	out, ct := trimTransparent(buf.Bytes(), "image/png")
	cfg, err := png.DecodeConfig(bytes.NewReader(out))
	if err != nil || ct != "image/png" || cfg.Width != 80 || cfg.Height != 20 {
		t.Errorf("trimmed to %dx%d %s (%v), want 80x20 image/png", cfg.Width, cfg.Height, ct, err)
	}

	full := pngOf(t, 50, 20) // opaque gray, nothing to trim
	if out, _ := trimTransparent(full, "image/png"); !bytes.Equal(out, full) {
		t.Error("an image with nothing to trim was re-encoded")
	}
	if out, ct := trimTransparent(webpX(10, 10), "image/webp"); ct != "image/webp" || len(out) != 30 {
		t.Error("a WebP was touched")
	}
}
