package service

import (
	"bytes"
	"image"
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
		"script":              svgWith(`<script>alert(1)</script>`),
		"SCRIPT":              svgWith(`<SCRIPT>alert(1)</SCRIPT>`),
		"prefixed script":     svgWith(`<s:script xmlns:s="http://www.w3.org/2000/svg">alert(1)</s:script>`),
		"prefixed iframe":     svgWith(`<h:iframe xmlns:h="http://www.w3.org/1999/xhtml" src="https://evil"/>`),
		"onload":              []byte(`<svg xmlns="http://www.w3.org/2000/svg" onload="alert(1)"></svg>`),
		"ONLOAD":              []byte(`<svg xmlns="http://www.w3.org/2000/svg" ONLOAD="alert(1)"></svg>`),
		"char-ref javascript": svgWith(`<use href="&#106;avascript:alert(1)"/>`),
		"char-ref https use":  svgWith(`<use href="&#104;ttps://evil/x.svg#a"/>`),
		"remote xlink":        svgWith(`<use xlink:href="//evil/x.svg#a"/>`),
		"anchor":              svgWith(`<a href="#x"><text>x</text></a>`),
		"animate href":        svgWith(`<animate attributeName="href" values="javascript:alert(1)"/>`),
		"image":               svgWith(`<image href="#x"/>`),
		"foreignObject":       svgWith(`<foreignObject><div>x</div></foreignObject>`),
		"style element":       svgWith(`<style>@import url(https://evil/x.css);</style>`),
		"style attribute":     svgWith(`<rect style="fill:url(https://evil/x)"/>`),
		"remote url fill":     svgWith(`<rect fill="url(https://evil/x#g)"/>`),
		"quoted remote url":   svgWith(`<rect fill="url('//evil/x#g')"/>`),
		"doctype entity":      []byte(`<!DOCTYPE svg [<!ENTITY x "y">]>` + svgOpen + `</svg>`),
		"stylesheet PI":       []byte(`<?xml-stylesheet href="https://evil/x.css"?>` + svgOpen + `</svg>`),
		"latin-1 encoding":    []byte(`<?xml version="1.0" encoding="ISO-8859-1"?>` + svgOpen + `</svg>`),
		"utf-16":              utf16le(svgOpen + `<script>alert(1)</script></svg>`),
		"two roots":           []byte(svgOpen + `</svg>` + svgOpen + `</svg>`),
		"not well formed":     []byte(svgOpen + `<rect>`),
		"too complex":         svgWith(string(bytes.Repeat([]byte(`<g/>`), maxSVGElements+1))),
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
