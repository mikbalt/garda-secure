// Package imgmeta removes metadata (EXIF incl. GPS, XMP, IPTC, comments,
// text chunks) from uploaded images without re-encoding them (BE-S9, UU PDP).
// JPEG orientation is preserved through a minimal EXIF block holding only the
// Orientation tag, so photos are not shown rotated.
package imgmeta

import (
	"bytes"
	"encoding/binary"
	"errors"
)

// ErrInvalid reports an image that could not be parsed.
var ErrInvalid = errors.New("invalid or unsupported image")

// Strip returns a copy of an image without metadata. contentType must be
// image/jpeg, image/png or image/webp.
func Strip(data []byte, contentType string) ([]byte, error) {
	switch contentType {
	case "image/jpeg":
		return stripJPEG(data)
	case "image/png":
		return stripPNG(data)
	case "image/webp":
		return stripWebP(data)
	}
	return nil, ErrInvalid
}

// ── JPEG ─────────────────────────────────────────────────────────────────────

const (
	mSOI  = 0xD8
	mEOI  = 0xD9
	mSOS  = 0xDA
	mAPP0 = 0xE0
	mAPP1 = 0xE1
	mAPP2 = 0xE2 // ICC profile: kept (no personal data, needed for colours)
	mAPPE = 0xEE // Adobe: kept (colour transform)
	mAPPD = 0xED // Photoshop/IPTC
	mCOM  = 0xFE
)

func stripJPEG(b []byte) ([]byte, error) {
	if len(b) < 4 || b[0] != 0xFF || b[1] != mSOI {
		return nil, ErrInvalid
	}
	type segment struct {
		marker byte
		data   []byte // length bytes + payload
	}
	var kept []segment
	var orientation uint16
	i := 2
	for i < len(b) {
		if b[i] != 0xFF {
			return nil, ErrInvalid
		}
		for i < len(b) && b[i] == 0xFF { // fill bytes
			i++
		}
		if i >= len(b) {
			return nil, ErrInvalid
		}
		marker := b[i]
		i++
		switch {
		case marker == mEOI:
			return nil, ErrInvalid // no image data
		case marker == 0x01 || (marker >= 0xD0 && marker <= 0xD7): // no length
			kept = append(kept, segment{marker: marker})
			continue
		}
		if i+2 > len(b) {
			return nil, ErrInvalid
		}
		n := int(binary.BigEndian.Uint16(b[i:]))
		if n < 2 || i+n > len(b) {
			return nil, ErrInvalid
		}
		seg := b[i : i+n]
		i += n
		if marker == mSOS {
			// Entropy-coded data and later scans run to EOI; anything after
			// EOI (vendor trailers) is dropped.
			rest := b[i:]
			end := bytes.Index(rest, []byte{0xFF, mEOI})
			if end < 0 {
				return nil, ErrInvalid
			}
			out := make([]byte, 0, len(b))
			out = append(out, 0xFF, mSOI)
			wroteExif := orientation < 2 || orientation > 8
			for k, s := range kept {
				if !wroteExif && (k > 0 || s.marker != mAPP0) {
					out = append(out, orientationAPP1(orientation)...)
					wroteExif = true
				}
				out = append(out, 0xFF, s.marker)
				out = append(out, s.data...)
			}
			if !wroteExif {
				out = append(out, orientationAPP1(orientation)...)
			}
			out = append(out, 0xFF, mSOS)
			out = append(out, seg...)
			out = append(out, rest[:end+2]...)
			return out, nil
		}
		payload := seg[2:]
		switch {
		case marker == mAPP1:
			if bytes.HasPrefix(payload, []byte("Exif\x00\x00")) {
				orientation = exifOrientation(payload[6:])
			}
			continue // EXIF (GPS, device, time) and XMP
		case marker == mAPPD, marker == mCOM:
			continue
		case marker > mAPP2 && marker <= 0xEF && marker != mAPPE:
			continue // vendor metadata segments
		}
		kept = append(kept, segment{marker: marker, data: seg})
	}
	return nil, ErrInvalid
}

// exifOrientation reads tag 0x0112 from IFD0 of a TIFF block; 0 if absent.
func exifOrientation(t []byte) uint16 {
	if len(t) < 8 {
		return 0
	}
	var bo binary.ByteOrder
	switch string(t[:2]) {
	case "II":
		bo = binary.LittleEndian
	case "MM":
		bo = binary.BigEndian
	default:
		return 0
	}
	if bo.Uint16(t[2:]) != 42 {
		return 0
	}
	off := int(bo.Uint32(t[4:]))
	if off < 8 || off+2 > len(t) {
		return 0
	}
	count := int(bo.Uint16(t[off:]))
	for e := 0; e < count; e++ {
		p := off + 2 + e*12
		if p+12 > len(t) {
			return 0
		}
		if bo.Uint16(t[p:]) == 0x0112 && bo.Uint16(t[p+2:]) == 3 {
			return bo.Uint16(t[p+8:])
		}
	}
	return 0
}

// orientationAPP1 is an APP1 segment whose EXIF holds only Orientation.
func orientationAPP1(o uint16) []byte {
	seg := []byte{
		0xFF, mAPP1, 0, 34, // marker, length (2 + 6 + 26)
		'E', 'x', 'i', 'f', 0, 0,
		'M', 'M', 0, 42, 0, 0, 0, 8, // TIFF header, IFD0 at offset 8
		0, 1, // one entry
		0x01, 0x12, 0, 3, 0, 0, 0, 1, 0, 0, 0, 0, // Orientation, SHORT, count 1, value below
		0, 0, 0, 0, // no next IFD
	}
	binary.BigEndian.PutUint16(seg[28:], o)
	return seg
}

// ── PNG ──────────────────────────────────────────────────────────────────────

var pngSig = []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1A, '\n'}

// pngDrop are ancillary chunks that carry metadata.
var pngDrop = map[string]bool{"eXIf": true, "tEXt": true, "zTXt": true, "iTXt": true, "tIME": true}

func stripPNG(b []byte) ([]byte, error) {
	if !bytes.HasPrefix(b, pngSig) {
		return nil, ErrInvalid
	}
	out := append(make([]byte, 0, len(b)), pngSig...)
	i := len(pngSig)
	for i+12 <= len(b) {
		n := int(binary.BigEndian.Uint32(b[i:]))
		if n < 0 || n > len(b)-i-12 {
			return nil, ErrInvalid
		}
		typ := string(b[i+4 : i+8])
		chunk := b[i : i+12+n]
		i += 12 + n
		if !pngDrop[typ] {
			out = append(out, chunk...)
		}
		if typ == "IEND" {
			return out, nil
		}
	}
	return nil, ErrInvalid
}

// ── WebP ─────────────────────────────────────────────────────────────────────

func stripWebP(b []byte) ([]byte, error) {
	if len(b) < 12 || string(b[:4]) != "RIFF" || string(b[8:12]) != "WEBP" {
		return nil, ErrInvalid
	}
	end := 8 + int(binary.LittleEndian.Uint32(b[4:]))
	if end > len(b) || end < 12 {
		return nil, ErrInvalid
	}
	out := append(make([]byte, 0, len(b)), b[:12]...)
	for i := 12; i < end; {
		if i+8 > end {
			return nil, ErrInvalid
		}
		fourcc := string(b[i : i+4])
		n := int(binary.LittleEndian.Uint32(b[i+4:]))
		total := 8 + n + n%2
		if n < 0 || i+8+n > end {
			return nil, ErrInvalid
		}
		if i+total > end {
			total = end - i
		}
		chunk := b[i : i+total]
		i += total
		switch fourcc {
		case "EXIF", "XMP ":
			continue
		case "VP8X":
			if n < 1 {
				return nil, ErrInvalid
			}
			c := append([]byte(nil), chunk...)
			c[8] &^= 0x08 | 0x04 // clear EXIF and XMP flags
			chunk = c
		}
		out = append(out, chunk...)
	}
	binary.LittleEndian.PutUint32(out[4:], uint32(len(out)-8)) // #nosec G115 -- shorter than the input, whose size fit in uint32
	return out, nil
}
