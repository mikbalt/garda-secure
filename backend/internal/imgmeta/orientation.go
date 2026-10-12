package imgmeta

import (
	"bytes"
	"encoding/binary"
)

// JPEGOrientation returns the EXIF Orientation (1-8) of a JPEG, or 1 when
// absent or not a JPEG. Values 5-8 mean the displayed image is rotated by
// 90 degrees (width and height swap).
func JPEGOrientation(b []byte) uint16 {
	if len(b) < 4 || b[0] != 0xFF || b[1] != mSOI {
		return 1
	}
	i := 2
	for i+4 <= len(b) {
		if b[i] != 0xFF {
			return 1
		}
		marker := b[i+1]
		i += 2
		if marker == mSOS || marker == mEOI {
			return 1
		}
		if marker == 0x01 || (marker >= 0xD0 && marker <= 0xD7) || marker == 0xFF {
			continue
		}
		n := int(binary.BigEndian.Uint16(b[i:]))
		if n < 2 || i+n > len(b) {
			return 1
		}
		payload := b[i+2 : i+n]
		if marker == mAPP1 && bytes.HasPrefix(payload, []byte("Exif\x00\x00")) {
			if o := exifOrientation(payload[6:]); o >= 1 && o <= 8 {
				return o
			}
			return 1
		}
		i += n
	}
	return 1
}
