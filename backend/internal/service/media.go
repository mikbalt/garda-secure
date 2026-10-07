package service

import (
	"regexp"
	"strings"
	"time"
)

// Media kinds accepted by POST /media; they become part of the object key.
var MediaKinds = []string{"selfie", "checkpoint", "incident", "defect", "visitor"}

// MaxIncidentPhotos is the PRD limit on photos per incident (FR-INC-01).
const MaxIncidentPhotos = 5

var objectKeyRe = regexp.MustCompile(`^sites/[0-9a-f-]{36}/(selfie|checkpoint|incident|defect|visitor)/[0-9a-f-]{36}\.(jpg|png|webp)$`)

// ObjectKey builds the storage key of an uploaded photo (D-012).
func ObjectKey(siteID, kind, id, ext string) string {
	return "sites/" + siteID + "/" + kind + "/" + strings.ToLower(id) + "." + ext
}

// CheckObjectKey verifies an optional photo key belongs to the site. Keys may
// reference photos not uploaded yet (offline capture, FR-OFF-05).
func CheckObjectKey(key *string, siteID string) error {
	if key == nil || *key == "" {
		return nil
	}
	if !objectKeyRe.MatchString(*key) || !strings.HasPrefix(*key, "sites/"+siteID+"/") {
		return BadRequest("invalid_object_key", "photo key %q is not a valid key for this site", *key)
	}
	return nil
}

// CheckObjectKeys verifies a list of photo keys.
func CheckObjectKeys(keys []string, siteID string) error {
	for i := range keys {
		if err := CheckObjectKey(&keys[i], siteID); err != nil {
			return err
		}
	}
	return nil
}

// BackupObjectKey names a database dump in the backups bucket.
func BackupObjectKey(t time.Time) string {
	return "postgres/securepatrol-" + t.UTC().Format("20060102-150405") + ".dump"
}
