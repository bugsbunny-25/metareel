package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/gif" // decode GIF uploads
	"image/jpeg"
	"image/png"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp" // decode WebP uploads
)

const (
	// MaxDiscImageBytes bounds an uploaded or fetched picture: a phone photo
	// of a cover is a few MB.
	MaxDiscImageBytes = 25 << 20
	// maxDiscImagePixels bounds the decoded bitmap (a 50 MP photo is ~200 MB
	// in memory), so a small file can't decode to something huge.
	maxDiscImagePixels = 50_000_000
	// discImageSide and discThumbSide are the longest side stored: big
	// enough to read a back cover's spec block, and a shelf thumbnail.
	discImageSide = 2000
	discThumbSide = 480
)

var errBadDiscImage = &ValidationError{Message: "the picture must be a JPEG, PNG, WebP or GIF image"}

// discImageNameRe is a stored file's name: <sha256>[_thumb].<jpg|png>.
var discImageNameRe = regexp.MustCompile(`^([0-9a-f]{64})(_thumb)?\.(jpg|png)$`)

// discImageSem lets one picture at a time be decoded and scaled: a phone
// photo's bitmap is large, and the server is a small home box.
var discImageSem = make(chan struct{}, 1)

// processedImage is a picture re-encoded for storage.
type processedImage struct {
	full, thumb   []byte
	contentType   string
	width, height int
	sha           string
}

// processDiscImage decodes an upload (whatever the client claimed it was),
// turns it upright from its EXIF orientation, scales it down, and re-encodes
// it: JPEG, or PNG when it has transparency (disc art). Re-encoding drops all
// metadata, including a phone photo's location.
func processDiscImage(ctx context.Context, data []byte) (processedImage, error) {
	switch {
	case len(data) == 0:
		return processedImage{}, &ValidationError{Message: "the picture is empty"}
	case len(data) > MaxDiscImageBytes:
		return processedImage{}, &ValidationError{Message: "the picture must be at most 25 MB"}
	}
	switch http.DetectContentType(data) {
	case "image/jpeg", "image/png", "image/gif", "image/webp":
	default:
		return processedImage{}, errBadDiscImage
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return processedImage{}, errBadDiscImage
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width*cfg.Height > maxDiscImagePixels {
		return processedImage{}, &ValidationError{Message: "the picture is too large in pixels (at most 50 megapixels)"}
	}

	select {
	case discImageSem <- struct{}{}:
	case <-ctx.Done():
		return processedImage{}, ctx.Err()
	}
	defer func() { <-discImageSem }()

	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return processedImage{}, errBadDiscImage
	}
	alpha := hasAlpha(src)
	// Scale first, then turn upright: the bound is square, so the order
	// doesn't change the result, and rotating the small image is cheap.
	full := orient(scaleDown(src, discImageSide), jpegOrientation(data))
	thumb := scaleDown(full, discThumbSide)
	out := processedImage{width: full.Bounds().Dx(), height: full.Bounds().Dy(), contentType: "image/jpeg"}
	if alpha {
		out.contentType = "image/png"
	}
	if out.full, err = encodeDiscImage(full, alpha, 88); err != nil {
		return processedImage{}, err
	}
	if out.thumb, err = encodeDiscImage(thumb, alpha, 82); err != nil {
		return processedImage{}, err
	}
	sum := sha256.Sum256(out.full)
	out.sha = hex.EncodeToString(sum[:])
	return out, nil
}

func encodeDiscImage(img image.Image, alpha bool, quality int) ([]byte, error) {
	var buf bytes.Buffer
	var err error
	if alpha {
		enc := png.Encoder{CompressionLevel: png.BestCompression}
		err = enc.Encode(&buf, img)
	} else {
		err = jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality})
	}
	if err != nil {
		return nil, fmt.Errorf("encode picture: %w", err)
	}
	return buf.Bytes(), nil
}

// scaleDown fits img within side×side, never enlarging it. The result is
// always a fresh RGBA so later steps never alias the decoder's buffer.
func scaleDown(img image.Image, side int) *image.RGBA {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w > side || h > side {
		if w >= h {
			h = max(1, h*side/w)
			w = side
		} else {
			w = max(1, w*side/h)
			h = side
		}
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	if w == b.Dx() && h == b.Dy() {
		draw.Draw(dst, dst.Bounds(), img, b.Min, draw.Src)
		return dst
	}
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), img, b, xdraw.Src, nil)
	return dst
}

// hasAlpha reports whether any pixel is not fully opaque. Only images whose
// colour model can carry alpha are checked.
func hasAlpha(img image.Image) bool {
	switch img.ColorModel() {
	case color.YCbCrModel, color.GrayModel, color.Gray16Model, color.CMYKModel:
		return false
	}
	if o, ok := img.(interface{ Opaque() bool }); ok {
		return !o.Opaque()
	}
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if _, _, _, a := img.At(x, y).RGBA(); a != 0xffff {
				return true
			}
		}
	}
	return false
}

// jpegOrientation reads the EXIF orientation (1-8) of a JPEG, or 1. Phones
// store photos sideways and set this tag instead of rotating the pixels.
func jpegOrientation(data []byte) int {
	if len(data) < 4 || data[0] != 0xFF || data[1] != 0xD8 {
		return 1
	}
	i := 2
	for i+4 <= len(data) {
		if data[i] != 0xFF {
			return 1
		}
		marker := data[i+1]
		if marker == 0xDA || marker == 0xD9 { // start of scan / end: no EXIF before the pixels
			return 1
		}
		size := int(binary.BigEndian.Uint16(data[i+2 : i+4]))
		if size < 2 || i+2+size > len(data) {
			return 1
		}
		seg := data[i+4 : i+2+size]
		if marker == 0xE1 && len(seg) > 14 && string(seg[:6]) == "Exif\x00\x00" {
			return exifOrientation(seg[6:])
		}
		i += 2 + size
	}
	return 1
}

func exifOrientation(tiff []byte) int {
	if len(tiff) < 8 {
		return 1
	}
	var bo binary.ByteOrder
	switch string(tiff[:2]) {
	case "II":
		bo = binary.LittleEndian
	case "MM":
		bo = binary.BigEndian
	default:
		return 1
	}
	ifd := int(bo.Uint32(tiff[4:8]))
	if ifd < 8 || ifd+2 > len(tiff) {
		return 1
	}
	n := int(bo.Uint16(tiff[ifd : ifd+2]))
	for k := 0; k < n; k++ {
		e := ifd + 2 + k*12
		if e+12 > len(tiff) {
			return 1
		}
		if bo.Uint16(tiff[e:e+2]) == 0x0112 {
			v := int(bo.Uint16(tiff[e+8 : e+10]))
			if v >= 1 && v <= 8 {
				return v
			}
			return 1
		}
	}
	return 1
}

// orient applies an EXIF orientation to the pixels.
func orient(img image.Image, o int) image.Image {
	if o <= 1 || o > 8 {
		return img
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	dw, dh := w, h
	if o >= 5 {
		dw, dh = h, w
	}
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var dx, dy int
			switch o {
			case 2:
				dx, dy = w-1-x, y
			case 3:
				dx, dy = w-1-x, h-1-y
			case 4:
				dx, dy = x, h-1-y
			case 5:
				dx, dy = y, x
			case 6:
				dx, dy = h-1-y, x
			case 7:
				dx, dy = h-1-y, w-1-x
			case 8:
				dx, dy = y, w-1-x
			}
			dst.Set(dx, dy, img.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return dst
}

// discImageStore keeps picture files under a directory, named by content.
type discImageStore struct{ dir string }

func (s discImageStore) ext(contentType string) string {
	if contentType == "image/png" {
		return "png"
	}
	return "jpg"
}

func (s discImageStore) path(name string) string {
	return filepath.Join(s.dir, "discs", name[:2], name)
}

// save writes both sizes; an existing file (the same picture) is kept.
func (s discImageStore) save(p processedImage) error {
	ext := s.ext(p.contentType)
	for _, f := range []struct {
		name string
		data []byte
	}{{p.sha + "." + ext, p.full}, {p.sha + "_thumb." + ext, p.thumb}} {
		dst := s.path(f.name)
		if _, err := os.Stat(dst); err == nil {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return fmt.Errorf("picture dir: %w", err)
		}
		tmp, err := os.CreateTemp(filepath.Dir(dst), ".upload-*")
		if err != nil {
			return fmt.Errorf("picture temp file: %w", err)
		}
		_, werr := tmp.Write(f.data)
		cerr := tmp.Close()
		if werr == nil && cerr == nil {
			werr = os.Chmod(tmp.Name(), 0o644) // CreateTemp makes it 0600
		}
		if werr != nil || cerr != nil {
			_ = os.Remove(tmp.Name())
			return fmt.Errorf("write picture: %w", errors.Join(werr, cerr))
		}
		if err := os.Rename(tmp.Name(), dst); err != nil {
			_ = os.Remove(tmp.Name())
			return fmt.Errorf("store picture: %w", err)
		}
	}
	return nil
}

// remove deletes both sizes of a picture.
func (s discImageStore) remove(sha, contentType string) {
	ext := s.ext(contentType)
	_ = os.Remove(s.path(sha + "." + ext))
	_ = os.Remove(s.path(sha + "_thumb." + ext))
}

// removeAny deletes a picture's files whatever their type.
func (s discImageStore) removeAny(sha string) {
	s.remove(sha, "image/jpeg")
	s.remove(sha, "image/png")
}

// Open returns a stored file by its public name, or os.ErrNotExist.
func (s discImageStore) Open(name string) (*os.File, string, error) {
	m := discImageNameRe.FindStringSubmatch(name)
	if m == nil {
		return nil, "", os.ErrNotExist
	}
	ct := "image/jpeg"
	if m[3] == "png" {
		ct = "image/png"
	}
	f, err := os.Open(s.path(name))
	return f, ct, err
}

// DiscImageURL is where a stored picture is served.
func DiscImageURL(sha, contentType string, thumb bool) string {
	ext := discImageStore{}.ext(contentType)
	if thumb {
		return "/api/v1/disc-images/" + sha + "_thumb." + ext
	}
	return "/api/v1/disc-images/" + sha + "." + ext
}

// errPrivateAddress: a fetched picture's host resolved to a local address.
var errPrivateAddress = errors.New("refusing to fetch from a local or private address")

// publicOnlyDialer refuses to connect anywhere but the public internet, so a
// picture URL from a third-party lookup can't reach the LAN (or this box).
// The check runs on the address actually dialled, after DNS.
func publicOnlyDialer() *net.Dialer {
	return &net.Dialer{
		Timeout: 10 * time.Second,
		Control: func(_, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return err
			}
			ip := net.ParseIP(host)
			if ip == nil || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
				ip.IsMulticast() || ip.IsUnspecified() || ip.IsInterfaceLocalMulticast() {
				return errPrivateAddress
			}
			if ip4 := ip.To4(); ip4 != nil && ip4[0] == 100 && ip4[1]&0xC0 == 64 { // 100.64.0.0/10 (CGNAT, Tailscale)
				return errPrivateAddress
			}
			return nil
		},
	}
}

// imageFetcher downloads pictures from public URLs.
type imageFetcher struct {
	client    *http.Client
	userAgent string
}

func newImageFetcher(userAgent string) *imageFetcher {
	tr := &http.Transport{
		DialContext:           publicOnlyDialer().DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 15 * time.Second,
		MaxIdleConns:          4,
		IdleConnTimeout:       30 * time.Second,
	}
	return &imageFetcher{
		client: &http.Client{
			Transport: tr,
			Timeout:   30 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 3 {
					return errors.New("too many redirects")
				}
				if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
					return errors.New("redirect to a non-http URL")
				}
				return nil
			},
		},
		userAgent: userAgent,
	}
}

func (f *imageFetcher) fetch(ctx context.Context, raw string) ([]byte, error) {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, &ValidationError{Message: "the picture URL must be http(s)"}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	if f.userAgent != "" {
		req.Header.Set("User-Agent", f.userAgent)
	}
	req.Header.Set("Accept", "image/*")
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: fetch picture from %s: %v", ErrUpstream, u.Host, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: fetch picture from %s: status %d", ErrUpstream, u.Host, resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "" && !strings.HasPrefix(ct, "image/") && !strings.HasPrefix(ct, "application/octet-stream") {
		return nil, fmt.Errorf("%w: %s did not answer with a picture (%s)", ErrUpstream, u.Host, ct)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, MaxDiscImageBytes+1))
	if err != nil {
		return nil, fmt.Errorf("%w: read picture from %s: %v", ErrUpstream, u.Host, err)
	}
	return data, nil
}
