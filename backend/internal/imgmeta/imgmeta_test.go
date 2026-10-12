package imgmeta

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"
)

func testImage() image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 16, 8))
	for x := 0; x < 16; x++ {
		img.Set(x, x%8, color.RGBA{R: 200, A: 255})
	}
	return img
}

// exifWithGPS builds an APP1 EXIF segment (little endian) with Orientation
// and a fake GPS marker string in its payload.
func exifWithGPS(orientation uint16) []byte {
	tiff := []byte{'I', 'I', 42, 0, 8, 0, 0, 0, 1, 0}
	entry := make([]byte, 12)
	binary.LittleEndian.PutUint16(entry[0:], 0x0112)
	binary.LittleEndian.PutUint16(entry[2:], 3)
	binary.LittleEndian.PutUint32(entry[4:], 1)
	binary.LittleEndian.PutUint16(entry[8:], orientation)
	tiff = append(tiff, entry...)
	tiff = append(tiff, 0, 0, 0, 0)
	tiff = append(tiff, []byte("GPS-6.2088,106.8456-SECRET")...)
	payload := append([]byte("Exif\x00\x00"), tiff...)
	seg := []byte{0xFF, 0xE1, 0, 0}
	binary.BigEndian.PutUint16(seg[2:], uint16(len(payload)+2))
	return append(seg, payload...)
}

func TestStripJPEG(t *testing.T) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, testImage(), nil); err != nil {
		t.Fatal(err)
	}
	enc := buf.Bytes()
	// SOI + EXIF + XMP + COM + rest of the encoder output + trailer after EOI.
	seg := func(marker byte, payload string) []byte {
		b := []byte{0xFF, marker, 0, 0}
		binary.BigEndian.PutUint16(b[2:], uint16(len(payload)+2))
		return append(b, payload...)
	}
	xmp := seg(0xE1, "http://ns.adobe.com/xap/1.0/\x00<x:xmpmeta/>")
	com := seg(0xFE, "SECRET-COM")
	in := append([]byte{0xFF, 0xD8}, exifWithGPS(6)...)
	in = append(in, xmp...)
	in = append(in, com...)
	in = append(in, enc[2:]...)
	in = append(in, []byte("TRAILER-SECRET")...)

	out, err := Strip(in, "image/jpeg")
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"SECRET", "GPS", "ns.adobe.com"} {
		if bytes.Contains(out, []byte(s)) {
			t.Errorf("output still contains %q", s)
		}
	}
	if _, err := jpeg.Decode(bytes.NewReader(out)); err != nil {
		t.Fatalf("stripped JPEG does not decode: %v", err)
	}
	i := bytes.Index(out, []byte("Exif\x00\x00"))
	if i < 0 || exifOrientation(out[i+6:]) != 6 {
		t.Fatalf("orientation not preserved")
	}
	again, err := Strip(out, "image/jpeg")
	if err != nil || !bytes.Equal(again, out) {
		t.Fatal("stripping is not idempotent")
	}
	if _, err := Strip([]byte("\xFF\xD8garbage"), "image/jpeg"); err == nil {
		t.Fatal("garbage accepted")
	}

	plain, err := Strip(enc, "image/jpeg")
	if err != nil || bytes.Contains(plain, []byte("Exif")) {
		t.Fatalf("plain JPEG: %v (EXIF added without orientation?)", err)
	}
}

func TestStripPNG(t *testing.T) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, testImage()); err != nil {
		t.Fatal(err)
	}
	enc := buf.Bytes()
	text := append([]byte("tEXt"), []byte("Location\x00SECRET-PLACE")...)
	chunk := make([]byte, 4)
	binary.BigEndian.PutUint32(chunk, uint32(len(text)-4))
	chunk = append(chunk, text...)
	chunk = binary.BigEndian.AppendUint32(chunk, crc32.ChecksumIEEE(text))
	iend := len(enc) - 12
	in := append(append(append([]byte{}, enc[:iend]...), chunk...), enc[iend:]...)
	if _, err := png.Decode(bytes.NewReader(in)); err != nil {
		t.Fatalf("test PNG invalid: %v", err)
	}
	out, err := Strip(in, "image/png")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(out, []byte("SECRET")) {
		t.Error("tEXt chunk kept")
	}
	if _, err := png.Decode(bytes.NewReader(out)); err != nil {
		t.Fatalf("stripped PNG does not decode: %v", err)
	}
}

func TestStripWebP(t *testing.T) {
	chunk := func(fourcc string, data []byte) []byte {
		c := append([]byte(fourcc), 0, 0, 0, 0)
		binary.LittleEndian.PutUint32(c[4:], uint32(len(data)))
		c = append(c, data...)
		if len(data)%2 == 1 {
			c = append(c, 0)
		}
		return c
	}
	body := []byte("WEBP")
	body = append(body, chunk("VP8X", []byte{0x08 | 0x04 | 0x10, 0, 0, 0, 0, 0, 0, 0, 0, 0})...)
	body = append(body, chunk("VP8L", []byte{1, 2, 3})...)
	body = append(body, chunk("EXIF", []byte("SECRET-EXIF"))...)
	body = append(body, chunk("XMP ", []byte("SECRET-XMP"))...)
	in := append([]byte("RIFF\x00\x00\x00\x00"), body...)
	binary.LittleEndian.PutUint32(in[4:], uint32(len(body)))

	out, err := Strip(in, "image/webp")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(out, []byte("SECRET")) {
		t.Error("metadata chunk kept")
	}
	if out[20]&(0x08|0x04) != 0 || out[20]&0x10 == 0 {
		t.Errorf("VP8X flags = %08b", out[20])
	}
	if int(binary.LittleEndian.Uint32(out[4:])) != len(out)-8 {
		t.Error("RIFF size not updated")
	}
}
