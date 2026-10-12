package export

import (
	"bytes"
	"errors"
	"image"
	"image/jpeg"
	_ "image/png" // decoder registration
	"net/http"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp" // decoder registration

	"github.com/securepatrol/backend/internal/imgmeta"
)

// MaxImagePixels guards against decompression bombs (40 MP).
const MaxImagePixels = 40_000_000

// ErrImage reports an image that cannot be embedded.
var ErrImage = errors.New("unsupported or oversized image")

// ImageConfig returns the displayed size of a JPEG/PNG/WebP image (EXIF
// orientation applied: rotated JPEGs swap width and height).
func ImageConfig(data []byte) (w, h int, err error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return 0, 0, ErrImage
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width*cfg.Height > MaxImagePixels {
		return 0, 0, ErrImage
	}
	w, h = cfg.Width, cfg.Height
	if http.DetectContentType(data) == "image/jpeg" && imgmeta.JPEGOrientation(data) >= 5 {
		w, h = h, w
	}
	return w, h, nil
}

// PrepareImage decodes a JPEG/PNG/WebP photo, applies its EXIF orientation,
// scales it to at most maxPx on the longer side and re-encodes it as JPEG
// (fpdf embeds JPEG natively; WebP is not supported by PDF readers).
func PrepareImage(data []byte, maxPx int) ([]byte, int, int, error) {
	if _, _, err := ImageConfig(data); err != nil {
		return nil, 0, 0, err
	}
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, 0, 0, ErrImage
	}
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w > maxPx || h > maxPx {
		if w >= h {
			h, w = h*maxPx/w, maxPx
		} else {
			w, h = w*maxPx/h, maxPx
		}
		if w < 1 {
			w = 1
		}
		if h < 1 {
			h = 1
		}
	}
	scaled := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.ApproxBiLinear.Scale(scaled, scaled.Bounds(), src, b, draw.Src, nil)
	var dst image.Image = scaled
	if http.DetectContentType(data) == "image/jpeg" { // rotate after scaling: cheaper
		dst = orient(scaled, imgmeta.JPEGOrientation(data))
	}
	var out bytes.Buffer
	if err := jpeg.Encode(&out, dst, &jpeg.Options{Quality: 80}); err != nil {
		return nil, 0, 0, err
	}
	db := dst.Bounds()
	return out.Bytes(), db.Dx(), db.Dy(), nil
}

// orient applies an EXIF orientation (2-8) to an image.
func orient(src image.Image, o uint16) image.Image {
	if o < 2 || o > 8 {
		return src
	}
	b := src.Bounds()
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
			case 2: // mirror horizontal
				dx, dy = w-1-x, y
			case 3: // rotate 180
				dx, dy = w-1-x, h-1-y
			case 4: // mirror vertical
				dx, dy = x, h-1-y
			case 5: // transpose
				dx, dy = y, x
			case 6: // rotate 90 CW
				dx, dy = h-1-y, x
			case 7: // transverse
				dx, dy = h-1-y, w-1-x
			case 8: // rotate 270 CW
				dx, dy = y, w-1-x
			}
			dst.Set(dx, dy, src.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return dst
}
