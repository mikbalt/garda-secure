//go:build integration

package integration

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/securepatrol/backend/internal/config"
	"github.com/securepatrol/backend/internal/middleware"
	"github.com/securepatrol/backend/internal/service"
)

// fakeS3 is just enough of the S3 API for HEAD and (conditional) PUT.
type fakeS3 struct {
	mu      sync.Mutex
	objs    map[string]fakeObj
	deleted []string // paths of DELETE requests
}

type fakeObj struct {
	size int64
	body []byte
	meta http.Header
}

func (f *fakeS3) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	o, exists := f.objs[r.URL.Path]
	switch r.Method {
	case http.MethodHead:
		if !exists {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		for k, v := range o.meta {
			w.Header()[k] = v
		}
		w.Header().Set("Content-Length", strconv.FormatInt(o.size, 10))
		w.Header().Set("ETag", `"0123456789abcdef"`)
		w.Header().Set("Last-Modified", time.Now().UTC().Format(http.TimeFormat))
		w.Header().Set("Content-Type", "image/jpeg")
		w.WriteHeader(http.StatusOK)
	case http.MethodPut:
		if exists && r.Header.Get("If-None-Match") == "*" {
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusPreconditionFailed)
			fmt.Fprint(w, `<Error><Code>PreconditionFailed</Code><Message>exists</Message></Error>`)
			return
		}
		body, _ := io.ReadAll(r.Body)
		size := int64(len(body))
		if d := r.Header.Get("X-Amz-Decoded-Content-Length"); d != "" {
			size, _ = strconv.ParseInt(d, 10, 64)
			body = decodeAWSChunked(body)
		}
		meta := http.Header{}
		for k, v := range r.Header {
			if strings.HasPrefix(strings.ToLower(k), "x-amz-meta-") {
				meta[k] = v
			}
		}
		f.objs[r.URL.Path] = fakeObj{size: size, body: body, meta: meta}
		w.Header().Set("ETag", `"0123456789abcdef"`)
		w.WriteHeader(http.StatusOK)
	case http.MethodGet:
		if r.URL.Query().Has("versions") { // ListObjectVersions (bucket path)
			f.listVersions(w, r)
			return
		}
		if !exists {
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `<Error><Code>NoSuchKey</Code><Message>missing</Message></Error>`)
			return
		}
		w.Header().Set("Content-Length", strconv.Itoa(len(o.body)))
		w.Header().Set("ETag", `"0123456789abcdef"`)
		w.Header().Set("Last-Modified", time.Now().UTC().Format(http.TimeFormat))
		w.Header().Set("Content-Type", "image/jpeg")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(o.body)
	case http.MethodDelete:
		delete(f.objs, r.URL.Path)
		f.deleted = append(f.deleted, r.URL.Path)
		w.WriteHeader(http.StatusNoContent)
	default:
		w.WriteHeader(http.StatusNotImplemented)
	}
}

// listVersions answers ListObjectVersions with one version per object.
func (f *fakeS3) listVersions(w http.ResponseWriter, r *http.Request) {
	bucket := "/" + strings.Trim(r.URL.Path, "/") + "/"
	prefix := r.URL.Query().Get("prefix")
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?><ListVersionsResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">`)
	fmt.Fprintf(&b, `<Name>%s</Name><Prefix>%s</Prefix><MaxKeys>1000</MaxKeys><IsTruncated>false</IsTruncated>`, strings.Trim(bucket, "/"), prefix)
	for path, o := range f.objs {
		key, ok := strings.CutPrefix(path, bucket)
		if !ok || !strings.HasPrefix(key, prefix) {
			continue
		}
		fmt.Fprintf(&b, `<Version><Key>%s</Key><VersionId>v1</VersionId><IsLatest>true</IsLatest><LastModified>%s</LastModified><ETag>"0123456789abcdef"</ETag><Size>%d</Size><StorageClass>STANDARD</StorageClass></Version>`,
			key, time.Now().UTC().Format("2006-01-02T15:04:05.000Z"), o.size)
	}
	b.WriteString(`</ListVersionsResult>`)
	w.Header().Set("Content-Type", "application/xml")
	_, _ = w.Write([]byte(b.String()))
}

// decodeAWSChunked strips the aws-chunked framing of a streaming-signed PUT.
func decodeAWSChunked(b []byte) []byte {
	var out []byte
	for len(b) > 0 {
		nl := bytes.Index(b, []byte("\r\n"))
		if nl < 0 {
			break
		}
		header := string(b[:nl])
		sizeHex, _, _ := strings.Cut(header, ";")
		n, err := strconv.ParseInt(sizeHex, 16, 64)
		if err != nil || n == 0 || int(n)+nl+2 > len(b) {
			break
		}
		out = append(out, b[nl+2:nl+2+int(n)]...)
		b = b[nl+2+int(n):]
		b = bytes.TrimPrefix(b, []byte("\r\n"))
	}
	return out
}

// jpegWithGPS returns a JPEG carrying an EXIF block with a GPS marker.
func jpegWithGPS(t *testing.T, shade uint8) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	img.Set(1, 1, color.RGBA{R: shade, A: 255})
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	payload := append([]byte("Exif\x00\x00II*\x00\x08\x00\x00\x00\x00\x00\x00\x00\x00\x00"), []byte("GPS-6.2,106.8")...)
	seg := []byte{0xFF, 0xE1, 0, 0}
	binary.BigEndian.PutUint16(seg[2:], uint16(len(payload)+2))
	out := append([]byte{0xFF, 0xD8}, append(seg, payload...)...)
	return append(out, buf.Bytes()[2:]...)
}

func (e *env) upload(tablet, guard, kind, id string, data []byte) resp {
	e.t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	_ = mw.WriteField("kind", kind)
	_ = mw.WriteField("id", id)
	fw, _ := mw.CreateFormFile("file", "photo.jpg")
	_, _ = fw.Write(data)
	_ = mw.Close()
	req := httptest.NewRequest("POST", "/api/v1/media", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+tablet)
	req.Header.Set(middleware.GuardHeader, guard)
	res, err := e.app.Test(req, 30_000)
	if err != nil {
		e.t.Fatal(err)
	}
	raw, _ := io.ReadAll(res.Body)
	return resp{status: res.StatusCode, raw: raw}
}

// BE-S4 / BE-S9: write-once evidence photos, metadata stripped, per-kind
// read permissions.
func TestMediaWriteOnceAndRoles(t *testing.T) {
	s3 := &fakeS3{objs: map[string]fakeObj{}}
	srv := httptest.NewServer(s3)
	defer srv.Close()
	e := newEnv(t, func(c *config.Config) { c.MinioEndpoint = strings.TrimPrefix(srv.URL, "http://") })
	admin := e.web(realm3, "media-admin", "admin")
	f := e.newFieldSite(realm3, admin, "Media A")

	id := uuid.NewString()
	photo := jpegWithGPS(t, 10)
	e.expect(e.upload(f.tablet, f.guard, "incident", id, photo), 201, "upload")
	key := service.ObjectKey(f.site, "incident", id, "jpg")
	stored := s3.objs["/photos/"+key]
	if stored.size == 0 || bytes.Contains(stored.body, []byte("GPS-")) {
		t.Fatalf("stored object: size %d, GPS kept = %v", stored.size, bytes.Contains(stored.body, []byte("GPS-")))
	}
	e.expect(e.upload(f.tablet, f.guard, "incident", id, photo), 200, "identical retry")
	if r := e.upload(f.tablet, f.guard, "incident", id, jpegWithGPS(t, 200)); r.status != 409 || !strings.Contains(string(r.raw), "media_exists") {
		t.Fatalf("overwrite with another photo: %d %s", r.status, r.raw)
	}
	if r := e.upload(f.tablet, f.guard, "incident", uuid.NewString(), []byte("\xFF\xD8\xFF\xE0garbage")); r.status != 400 {
		t.Fatalf("broken JPEG: %d %s", r.status, r.raw)
	}

	selfie := service.ObjectKey(f.site, "selfie", uuid.NewString(), "jpg")
	visitor := service.ObjectKey(f.site, "visitor", uuid.NewString(), "jpg")
	cases := []struct {
		role, key string
		want      int
	}{
		{"client-readonly", key, 200},
		{"client-readonly", selfie, 403},
		{"client-readonly", visitor, 403},
		{"hr", selfie, 200},
		{"hr", key, 403},
		{"hr", visitor, 403},
		{"supervisor", visitor, 200},
		{"admin", selfie, 200},
	}
	for _, c := range cases {
		r := e.do("GET", "/api/v1/media/url?key="+c.key, e.web(realm3, "media-"+c.role, c.role), "", nil)
		if r.status != c.want {
			t.Errorf("%s reads %s: %d, want %d (%s)", c.role, c.key, r.status, c.want, r.raw)
		}
	}
	if p := e.do("GET", "/api/v1/media/url?key="+key, admin, "", nil).obj(); p["uploaded"] != true || p["url"] == "" {
		t.Fatalf("media url = %v", p)
	}
}
