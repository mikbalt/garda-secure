package handler

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/securepatrol/backend/internal/export"
	"github.com/securepatrol/backend/internal/imgmeta"
	"github.com/securepatrol/backend/internal/model"
	"github.com/securepatrol/backend/internal/repository"
	"github.com/securepatrol/backend/internal/service"
	"github.com/securepatrol/backend/internal/storage"
)

// maxFloorplansPerSite bounds uploads per site.
const maxFloorplansPerSite = 20

// Site floor plans (F3, D-035): an uploaded image per building/floor on which
// checkpoints are placed (plan_x/plan_y 0..1); the live view draws patrol
// progress on it without any online map tiles.

// GET /sites/:id/floorplans — floor plans with pre-signed image URLs.
func (h *Handler) listFloorplans(c *fiber.Ctx) error {
	site, err := h.siteFor(c, c.Params("id"))
	if err != nil {
		return h.fail(c, err)
	}
	rows, err := repository.ListFloorplans(c.Context(), h.DB, site.ID)
	if err != nil {
		return h.fail(c, err)
	}
	for i := range rows {
		rows[i].URL, _ = h.Storage.PresignGet(c.Context(), h.Cfg.MinioBucketPhotos, rows[i].ObjectKey, presignTTL)
	}
	return c.JSON(rows)
}

// POST /sites/:id/floorplans (admin, multipart: file, name) — JPEG/PNG/WebP
// ≤ 10 MB, metadata stripped, stored write-once under
// sites/<site>/floorplan/<uuid>.<ext>.
func (h *Handler) uploadFloorplan(c *fiber.Ctx) error {
	site, err := h.siteFor(c, c.Params("id"))
	if err != nil {
		return h.fail(c, err)
	}
	name := strings.TrimSpace(c.FormValue("name"))
	if name == "" {
		return h.fail(c, service.BadRequest("name_required", "name is required"))
	}
	if err := service.CheckLengths(service.S("name", name, service.MaxName)); err != nil {
		return h.fail(c, err)
	}
	existing, err := repository.ListFloorplans(c.Context(), h.DB, site.ID)
	if err != nil {
		return h.fail(c, err)
	}
	if len(existing) >= maxFloorplansPerSite {
		return h.fail(c, service.Conflict("too_many_floorplans", "a site can have at most %d floor plans", maxFloorplansPerSite))
	}
	fh, err := c.FormFile("file")
	if err != nil {
		return h.fail(c, service.BadRequest("file_required", "multipart field 'file' is required"))
	}
	if fh.Size > MaxPhotoBytes {
		return h.fail(c, service.BadRequest("file_too_large", "image exceeds %d MB", MaxPhotoBytes>>20))
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
	if len(data) > MaxPhotoBytes {
		return h.fail(c, service.BadRequest("file_too_large", "image exceeds %d MB", MaxPhotoBytes>>20))
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
	w, hh, err := export.ImageConfig(clean)
	if err != nil {
		return h.fail(c, service.BadRequest("invalid_image", "the image could not be read or is too large"))
	}
	id := uuid.NewString()
	key := "sites/" + site.ID + "/floorplan/" + id + "." + ext
	sum := sha256.Sum256(clean)
	if err := h.Storage.PutNew(c.Context(), h.Cfg.MinioBucketPhotos, key, clean, ct, hex.EncodeToString(sum[:])); err != nil {
		if errors.Is(err, storage.ErrExists) {
			return h.fail(c, service.Conflict("media_exists", "object already exists"))
		}
		return h.fail(c, err)
	}
	who := actor(c)
	fp, err := repository.CreateFloorplan(c.Context(), h.DB, model.Floorplan{
		ID: id, SiteID: site.ID, Name: name, ObjectKey: key, ContentType: ct,
		Width: w, Height: hh, SizeBytes: int64(len(clean)), CreatedBy: &who,
	})
	if err != nil {
		return h.fail(c, err)
	}
	fp.URL, _ = h.Storage.PresignGet(c.Context(), h.Cfg.MinioBucketPhotos, key, presignTTL)
	return c.Status(fiber.StatusCreated).JSON(fp)
}

// DELETE /sites/:id/floorplans/:floorplanId (admin) — only when no
// checkpoint of the site's routes is placed on it (409 floorplan_in_use).
func (h *Handler) deleteFloorplan(c *fiber.Ctx) error {
	site, err := h.siteFor(c, c.Params("id"))
	if err != nil {
		return h.fail(c, err)
	}
	fp, err := repository.GetFloorplan(c.Context(), h.DB, c.Params("floorplanId"))
	if err != nil {
		return h.fail(c, err)
	}
	if fp.SiteID != site.ID {
		return h.fail(c, repository.ErrNotFound)
	}
	used, err := repository.FloorplanInUse(c.Context(), h.DB, site.ID, fp.ID)
	if err != nil {
		return h.fail(c, err)
	}
	if used {
		return h.fail(c, service.Conflict("floorplan_in_use", "checkpoints are placed on this floor plan; move them first"))
	}
	if err := repository.DeleteFloorplan(c.Context(), h.DB, fp.ID); err != nil {
		return h.fail(c, err)
	}
	if err := h.Storage.RemoveAllVersions(c.Context(), h.Cfg.MinioBucketPhotos, fp.ObjectKey); err != nil {
		// The metadata is gone; an orphaned image is harmless.
		h.Log.Warn().Err(err).Str("key", fp.ObjectKey).Msg("floor plan image not deleted")
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// checkRoutePlans validates checkpoint plan positions and that their floor
// plans belong to the route's site.
func (h *Handler) checkRoutePlans(c *fiber.Ctx, r *model.Route) error {
	ids, err := service.CheckCheckpointPlans(r.Checkpoints)
	if err != nil || len(ids) == 0 {
		return err
	}
	missing, err := repository.MissingFloorplans(c.Context(), h.DB, r.SiteID, ids)
	if err != nil {
		return err
	}
	if len(missing) > 0 {
		return service.BadRequest("unknown_floorplan", "floor plan %s does not belong to this site", missing[0])
	}
	return nil
}
