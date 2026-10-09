package handler

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"slices"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/securepatrol/backend/internal/imgmeta"
	"github.com/securepatrol/backend/internal/middleware"
	"github.com/securepatrol/backend/internal/service"
	"github.com/securepatrol/backend/internal/storage"
)

// mediaReadRoles lists which web roles may open photos of each kind
// (BE-S9): the roles that can read the records the photos belong to.
var mediaReadRoles = map[string][]string{
	"selfie":     {middleware.RoleAdmin, middleware.RoleSupervisor, middleware.RoleHR},
	"visitor":    {middleware.RoleAdmin, middleware.RoleSupervisor},
	"defect":     {middleware.RoleAdmin, middleware.RoleSupervisor},
	"incident":   {middleware.RoleAdmin, middleware.RoleSupervisor, middleware.RoleClientReadonly},
	"checkpoint": {middleware.RoleAdmin, middleware.RoleSupervisor, middleware.RoleClientReadonly},
}

// GET /media/url?key= — pre-signed URL for a stored photo of a site the
// caller can read, if their role may see that kind of photo.
func (h *Handler) mediaURL(c *fiber.Ctx) error {
	key := c.Query("key")
	parts := strings.SplitN(key, "/", 4) // sites/<site>/<kind>/<file>
	if len(parts) < 4 || parts[0] != "sites" {
		return h.fail(c, service.BadRequest("invalid_object_key", "invalid photo key"))
	}
	if err := service.CheckObjectKey(&key, parts[1]); err != nil {
		return h.fail(c, err)
	}
	if _, err := h.siteFor(c, parts[1]); err != nil {
		return h.fail(c, err)
	}
	if !middleware.GetPrincipal(c).HasRole(mediaReadRoles[parts[2]]...) {
		return h.fail(c, service.Forbidden("media_forbidden", "your role may not view %s photos", parts[2]))
	}
	return c.JSON(h.photos(c.Context(), key)[0])
}

// MaxPhotoBytes caps a single photo upload.
const MaxPhotoBytes = 10 << 20

var imageExt = map[string]string{"image/jpeg": "jpg", "image/png": "png", "image/webp": "webp"}

// POST /media (tablet, multipart) fields: file, kind, id? — returns the object
// key to reference from records. Metadata (EXIF/GPS, XMP, text chunks) is
// stripped before storing. Objects are write-once evidence (BE-S4): an
// identical re-upload (same size and SHA-256 after stripping) answers 200 so
// retries are safe; any other upload to an existing key gets 409
// media_exists.
func (h *Handler) uploadMedia(c *fiber.Ctx) error {
	site, err := h.deviceSite(c)
	if err != nil {
		return h.fail(c, err)
	}
	kind := c.FormValue("kind")
	if !slices.Contains(service.MediaKinds, kind) {
		return h.fail(c, service.BadRequest("invalid_kind", "kind must be one of %s", strings.Join(service.MediaKinds, ", ")))
	}
	id := c.FormValue("id")
	if id == "" {
		id = uuid.NewString()
	} else if err := optionalUUID(&id); err != nil {
		return h.fail(c, err)
	}
	fh, err := c.FormFile("file")
	if err != nil {
		return h.fail(c, service.BadRequest("file_required", "multipart field 'file' is required"))
	}
	if fh.Size > MaxPhotoBytes {
		return h.fail(c, service.BadRequest("file_too_large", "photo exceeds %d MB", MaxPhotoBytes>>20))
	}
	f, err := fh.Open()
	if err != nil {
		return h.fail(c, err)
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, MaxPhotoBytes+1))
	if err != nil {
		return h.fail(c, err)
	}
	ct := http.DetectContentType(data)
	ext, ok := imageExt[ct]
	if !ok {
		return h.fail(c, service.BadRequest("invalid_file_type", "only JPEG, PNG or WebP images are accepted"))
	}
	clean, err := imgmeta.Strip(data, ct)
	if err != nil {
		return h.fail(c, service.BadRequest("invalid_image", "the image could not be read"))
	}
	sum := sha256.Sum256(clean)
	hash := hex.EncodeToString(sum[:])
	key := service.ObjectKey(site.ID, kind, id, ext)
	out := fiber.Map{"object_key": key, "content_type": ct, "size": len(clean), "sha256": hash}

	bucket := h.Cfg.MinioBucketPhotos
	info, exists, err := h.Storage.Stat(c.Context(), bucket, key)
	if err != nil {
		return h.fail(c, err)
	}
	if !exists {
		err = h.Storage.PutNew(c.Context(), bucket, key, clean, ct, hash)
		if err == nil {
			return c.Status(fiber.StatusCreated).JSON(out)
		}
		if !errors.Is(err, storage.ErrExists) {
			return h.fail(c, err)
		}
		// Lost a race with a concurrent upload of the same key.
		if info, _, err = h.Storage.Stat(c.Context(), bucket, key); err != nil {
			return h.fail(c, err)
		}
	}
	if info.SHA256 == hash && info.Size == int64(len(clean)) {
		return c.JSON(out) // identical retry
	}
	return h.fail(c, service.Conflict("media_exists", "a different photo is already stored under this id"))
}
