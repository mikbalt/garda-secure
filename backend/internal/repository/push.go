package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/securepatrol/backend/internal/model"
)

// Push delivery channels in notification_log. They have no rule recipients:
// users subscribe their browsers, tablets register an FCM token.
const (
	ChannelWebPush = "webpush"
	ChannelFCM     = "fcm"
)

// pushChannels is used to keep push deliveries out of the rule delivery logs.
var pushChannels = []string{ChannelWebPush, ChannelFCM}

// ── Audience snapshots ───────────────────────────────────────────────────────

// sitesArg stores nil (no restriction) as NULL and a restriction (possibly
// to no site) as an array.
func sitesArg(sites []string) any {
	if sites == nil {
		return nil
	}
	return sites
}

func rolesArg(roles []string) []string {
	if roles == nil {
		return []string{}
	}
	return roles
}

// UpsertPushUser creates or refreshes a user's audience snapshot.
func UpsertPushUser(ctx context.Context, db DBTX, u model.PushUser) error {
	_, err := db.Exec(ctx, `
		INSERT INTO push_users (keycloak_realm, keycloak_user_id, username, roles, sites)
		VALUES ($1, $2, $3, $4, $5::uuid[])
		ON CONFLICT (keycloak_realm, keycloak_user_id) DO UPDATE
		  SET username = EXCLUDED.username, roles = EXCLUDED.roles, sites = EXCLUDED.sites,
		      seen_at = now(), updated_at = now()`,
		u.Realm, u.UserID, u.Username, rolesArg(u.Roles), sitesArg(u.Sites))
	if err != nil {
		return fmt.Errorf("UpsertPushUser: %w", err)
	}
	return nil
}

// RefreshPushUser updates an existing snapshot (no row is created for users
// who never used push).
func RefreshPushUser(ctx context.Context, db DBTX, u model.PushUser) error {
	_, err := db.Exec(ctx, `
		UPDATE push_users
		SET username = $3, roles = $4, sites = $5::uuid[], seen_at = now(), updated_at = now()
		WHERE keycloak_realm = $1 AND keycloak_user_id = $2`,
		u.Realm, u.UserID, u.Username, rolesArg(u.Roles), sitesArg(u.Sites))
	if err != nil {
		return fmt.Errorf("RefreshPushUser: %w", err)
	}
	return nil
}

// SetPushUserAudience replaces roles and sites of an existing snapshot after
// an admin changed the user (without touching seen_at).
func SetPushUserAudience(ctx context.Context, db DBTX, realm, userID string, roles, sites []string) error {
	_, err := db.Exec(ctx, `
		UPDATE push_users SET roles = $3, sites = $4::uuid[], updated_at = now()
		WHERE keycloak_realm = $1 AND keycloak_user_id = $2`, realm, userID, rolesArg(roles), sitesArg(sites))
	if err != nil {
		return fmt.Errorf("SetPushUserAudience: %w", err)
	}
	return nil
}

// DeletePushUser removes a user's snapshot, preferences and subscriptions.
func DeletePushUser(ctx context.Context, db DBTX, realm, userID string) error {
	_, err := db.Exec(ctx, `DELETE FROM push_users WHERE keycloak_realm = $1 AND keycloak_user_id = $2`, realm, userID)
	if err != nil {
		return fmt.Errorf("DeletePushUser: %w", err)
	}
	return nil
}

// GetPushPrefs returns a user's per-event overrides (empty map when none).
func GetPushPrefs(ctx context.Context, db DBTX, realm, userID string) (map[string]bool, error) {
	var raw []byte
	err := db.QueryRow(ctx, `SELECT prefs FROM push_users WHERE keycloak_realm = $1 AND keycloak_user_id = $2`,
		realm, userID).Scan(&raw)
	if err != nil {
		if mapNoRows(err) == ErrNotFound {
			return map[string]bool{}, nil
		}
		return nil, fmt.Errorf("GetPushPrefs: %w", err)
	}
	return decodePrefs(raw), nil
}

// MergePushPrefs merges overrides into a user's preferences (the snapshot
// row must exist) and returns the result.
func MergePushPrefs(ctx context.Context, db DBTX, realm, userID string, prefs map[string]bool) (map[string]bool, error) {
	b, err := json.Marshal(prefs)
	if err != nil {
		return nil, err
	}
	var raw []byte
	err = db.QueryRow(ctx, `
		UPDATE push_users SET prefs = prefs || $3::jsonb, updated_at = now()
		WHERE keycloak_realm = $1 AND keycloak_user_id = $2
		RETURNING prefs`, realm, userID, b).Scan(&raw)
	if err != nil {
		return nil, mapNoRows(err)
	}
	return decodePrefs(raw), nil
}

func decodePrefs(raw []byte) map[string]bool {
	out := map[string]bool{}
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil {
		return out
	}
	for k, v := range m {
		if b, ok := v.(bool); ok {
			out[k] = b
		}
	}
	return out
}

// ── Browser subscriptions ────────────────────────────────────────────────────

const pushSubColumns = `id, keycloak_realm, keycloak_user_id, kind, label, endpoint, p256dh, auth,
	created_at, last_success_at, last_failure_at, last_error`

func scanPushSub(row pgx.Row) (model.PushSubscription, error) {
	var s model.PushSubscription
	err := row.Scan(&s.ID, &s.Realm, &s.UserID, &s.Kind, &s.Label, &s.Endpoint, &s.P256dh, &s.Auth,
		&s.CreatedAt, &s.LastSuccessAt, &s.LastFailureAt, &s.LastError)
	return s, err
}

func scanPushSubRows(rows pgx.Rows) (model.PushSubscription, error) { return scanPushSub(rows) }

// SaveResult tells how SavePushSubscription stored an endpoint.
type SaveResult int

// SavePushSubscription outcomes.
const (
	SubCreated  SaveResult = iota // new endpoint
	SubExisting                   // the caller already had it (keys/label refreshed)
	SubRebound                    // it belonged to another user and now belongs to the caller
)

// SavePushSubscription stores a browser subscription for a user. An endpoint
// is unique: one already stored for another user (same browser profile, new
// login) is re-bound to the caller. Run it in a transaction.
func SavePushSubscription(ctx context.Context, db DBTX, s model.PushSubscription) (string, SaveResult, error) {
	var prevRealm, prevUser string
	err := db.QueryRow(ctx, `
		SELECT keycloak_realm, keycloak_user_id FROM push_subscriptions WHERE endpoint = $1 FOR UPDATE`,
		s.Endpoint).Scan(&prevRealm, &prevUser)
	res := SubCreated
	switch {
	case err == nil && prevRealm == s.Realm && prevUser == s.UserID:
		res = SubExisting
	case err == nil:
		res = SubRebound
	case mapNoRows(err) != ErrNotFound:
		return "", 0, fmt.Errorf("SavePushSubscription: %w", err)
	}
	var id string
	err = db.QueryRow(ctx, `
		INSERT INTO push_subscriptions (keycloak_realm, keycloak_user_id, kind, endpoint, p256dh, auth, label)
		VALUES ($1, $2, 'webpush', $3, $4, $5, $6)
		ON CONFLICT (endpoint) DO UPDATE
		  SET keycloak_realm = EXCLUDED.keycloak_realm, keycloak_user_id = EXCLUDED.keycloak_user_id,
		      p256dh = EXCLUDED.p256dh, auth = EXCLUDED.auth, label = EXCLUDED.label, updated_at = now(),
		      last_error = CASE WHEN push_subscriptions.keycloak_user_id = EXCLUDED.keycloak_user_id
		                        THEN push_subscriptions.last_error END
		RETURNING id`, s.Realm, s.UserID, s.Endpoint, s.P256dh, s.Auth, s.Label).Scan(&id)
	if err != nil {
		return "", 0, fmt.Errorf("SavePushSubscription: %w", err)
	}
	return id, res, nil
}

// CountPushSubscriptions returns how many subscriptions a user has.
func CountPushSubscriptions(ctx context.Context, db DBTX, realm, userID string) (int, error) {
	var n int
	err := db.QueryRow(ctx, `
		SELECT count(*) FROM push_subscriptions WHERE keycloak_realm = $1 AND keycloak_user_id = $2`,
		realm, userID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("CountPushSubscriptions: %w", err)
	}
	return n, nil
}

// ListPushSubscriptions returns a user's subscriptions, newest first.
func ListPushSubscriptions(ctx context.Context, db DBTX, realm, userID string) ([]model.PushSubscription, error) {
	rows, err := db.Query(ctx, `
		SELECT `+pushSubColumns+` FROM push_subscriptions
		WHERE keycloak_realm = $1 AND keycloak_user_id = $2
		ORDER BY created_at DESC, id`, realm, userID)
	if err != nil {
		return nil, fmt.Errorf("ListPushSubscriptions: %w", err)
	}
	return collect(rows, scanPushSubRows)
}

// GetPushSubscription retrieves a subscription by ID.
func GetPushSubscription(ctx context.Context, db DBTX, id string) (*model.PushSubscription, error) {
	s, err := scanPushSub(db.QueryRow(ctx, `SELECT `+pushSubColumns+` FROM push_subscriptions WHERE id = $1`, id))
	if err != nil {
		return nil, mapNoRows(err)
	}
	return &s, nil
}

// DeleteOwnPushSubscription removes a subscription of the given user;
// ErrNotFound when it is not theirs.
func DeleteOwnPushSubscription(ctx context.Context, db DBTX, id, realm, userID string) error {
	tag, err := db.Exec(ctx, `
		DELETE FROM push_subscriptions WHERE id = $1 AND keycloak_realm = $2 AND keycloak_user_id = $3`,
		id, realm, userID)
	if err != nil {
		return fmt.Errorf("DeleteOwnPushSubscription: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// DeletePushSubscription removes a subscription the push service reported
// gone (404/410).
func DeletePushSubscription(ctx context.Context, db DBTX, id string) error {
	if _, err := db.Exec(ctx, `DELETE FROM push_subscriptions WHERE id = $1`, id); err != nil {
		return fmt.Errorf("DeletePushSubscription: %w", err)
	}
	return nil
}

// MarkPushSubscription records the outcome of a send (failure = non-nil).
func MarkPushSubscription(ctx context.Context, db DBTX, id string, failure *string) error {
	var err error
	if failure == nil {
		_, err = db.Exec(ctx, `UPDATE push_subscriptions SET last_success_at = now(), last_error = NULL WHERE id = $1`, id)
	} else {
		_, err = db.Exec(ctx, `UPDATE push_subscriptions SET last_failure_at = now(), last_error = $2 WHERE id = $1`, id, *failure)
	}
	if err != nil {
		return fmt.Errorf("MarkPushSubscription: %w", err)
	}
	return nil
}

// PushTarget is one browser subscription with its user's audience snapshot.
type PushTarget struct {
	SubscriptionID string
	UserID         string
	Roles          []string
	Sites          []string // nil = every site of the realm
	Prefs          map[string]bool
}

// ListPushTargets returns the subscriptions of a realm's users who hold one
// of roles and were seen after seenAfter (fresh snapshots only).
func ListPushTargets(ctx context.Context, db DBTX, realm string, roles []string, seenAfter time.Time) ([]PushTarget, error) {
	rows, err := db.Query(ctx, `
		SELECT s.id, u.keycloak_user_id, u.roles, u.sites::text[], u.prefs
		FROM push_subscriptions s
		JOIN push_users u ON u.keycloak_realm = s.keycloak_realm AND u.keycloak_user_id = s.keycloak_user_id
		WHERE s.keycloak_realm = $1 AND u.roles && $2::text[] AND u.seen_at > $3
		ORDER BY s.created_at, s.id`, realm, rolesArg(roles), seenAfter)
	if err != nil {
		return nil, fmt.Errorf("ListPushTargets: %w", err)
	}
	return collect(rows, func(r pgx.Rows) (PushTarget, error) {
		var t PushTarget
		var raw []byte
		err := r.Scan(&t.SubscriptionID, &t.UserID, &t.Roles, &t.Sites, &raw)
		t.Prefs = decodePrefs(raw)
		return t, err
	})
}

// DeletePushDeliveries removes up to limit finished push deliveries created
// before cutoff (retention).
func DeletePushDeliveries(ctx context.Context, db DBTX, cutoff time.Time, limit int) (int64, error) {
	tag, err := db.Exec(ctx, `
		DELETE FROM notification_log
		WHERE id IN (SELECT id FROM notification_log
		             WHERE channel = ANY($3::text[]) AND status <> 'queued' AND created_at < $1 LIMIT $2)`,
		cutoff, limit, pushChannels)
	if err != nil {
		return 0, fmt.Errorf("DeletePushDeliveries: %w", err)
	}
	return tag.RowsAffected(), nil
}

// ── Tablet FCM tokens ────────────────────────────────────────────────────────

// SetDevicePushToken stores a tablet's FCM token. A token moves with the
// app installation, so it is removed from any other device first.
func SetDevicePushToken(ctx context.Context, db DBTX, deviceID, token, platform string) error {
	if _, err := db.Exec(ctx, `
		UPDATE devices SET push_token = NULL, push_platform = NULL, push_token_at = NULL
		WHERE push_token = $1 AND id <> $2`, token, deviceID); err != nil {
		return fmt.Errorf("SetDevicePushToken: %w", err)
	}
	tag, err := db.Exec(ctx, `
		UPDATE devices SET push_token = $2, push_platform = $3, push_token_at = now() WHERE id = $1`,
		deviceID, token, platform)
	if err != nil {
		return fmt.Errorf("SetDevicePushToken: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ClearDevicePushToken removes a tablet's FCM token.
func ClearDevicePushToken(ctx context.Context, db DBTX, deviceID string) error {
	if _, err := db.Exec(ctx, `
		UPDATE devices SET push_token = NULL, push_platform = NULL, push_token_at = NULL WHERE id = $1`,
		deviceID); err != nil {
		return fmt.Errorf("ClearDevicePushToken: %w", err)
	}
	return nil
}

// ClearPushToken removes an FCM token FCM reported unregistered.
func ClearPushToken(ctx context.Context, db DBTX, token string) error {
	if _, err := db.Exec(ctx, `
		UPDATE devices SET push_token = NULL, push_platform = NULL, push_token_at = NULL WHERE push_token = $1`,
		token); err != nil {
		return fmt.Errorf("ClearPushToken: %w", err)
	}
	return nil
}

// TabletTarget is a tablet with an FCM token.
type TabletTarget struct {
	DeviceID string
	Token    string
}

// ListSiteTabletTargets returns the active tablets of a site that have an
// FCM token, except exclude (the tablet that raised the event).
func ListSiteTabletTargets(ctx context.Context, db DBTX, siteID, exclude string) ([]TabletTarget, error) {
	rows, err := db.Query(ctx, `
		SELECT id, push_token FROM devices
		WHERE site_id = $1 AND status = 'active' AND push_token IS NOT NULL AND id::text <> $2
		ORDER BY id`, siteID, exclude)
	if err != nil {
		return nil, fmt.Errorf("ListSiteTabletTargets: %w", err)
	}
	return collect(rows, func(r pgx.Rows) (TabletTarget, error) {
		var t TabletTarget
		err := r.Scan(&t.DeviceID, &t.Token)
		return t, err
	})
}

// GetTabletTarget returns one tablet's FCM token (ErrNotFound without one).
func GetTabletTarget(ctx context.Context, db DBTX, deviceID string) (*TabletTarget, error) {
	var t TabletTarget
	err := db.QueryRow(ctx, `SELECT id, push_token FROM devices WHERE id = $1 AND push_token IS NOT NULL`,
		deviceID).Scan(&t.DeviceID, &t.Token)
	if err != nil {
		return nil, mapNoRows(err)
	}
	return &t, nil
}

// ── SOS escalation and tablet health ────────────────────────────────────────

// SosEscalation is an unacknowledged SOS that may need a reminder.
type SosEscalation struct {
	model.SosAlert
	Escalations int
}

// ListSosForEscalation returns unacknowledged SOS alerts received after
// since that have had fewer than max reminders.
func ListSosForEscalation(ctx context.Context, db DBTX, since time.Time, max int) ([]SosEscalation, error) {
	rows, err := db.Query(ctx, `
		SELECT `+sosColumns+`, escalations FROM sos_alerts
		WHERE acknowledged_at IS NULL AND escalations < $2 AND received_at > $1
		ORDER BY received_at`, since, max)
	if err != nil {
		return nil, fmt.Errorf("ListSosForEscalation: %w", err)
	}
	return collect(rows, func(r pgx.Rows) (SosEscalation, error) {
		var e SosEscalation
		a := &e.SosAlert
		err := r.Scan(&a.ID, &a.GuardID, &a.SiteID, &a.DeviceID, &a.FiredAt, &a.ReceivedAt,
			&a.GPSLat, &a.GPSLng, &a.AcknowledgedBy, &a.AcknowledgedAt, &a.Flags, &a.GuardName, &e.Escalations)
		if a.Flags == nil {
			a.Flags = []string{}
		}
		return e, err
	})
}

// SetSosEscalations records that reminder n was staged; false when the alert
// was acknowledged or already reminded meanwhile.
func SetSosEscalations(ctx context.Context, db DBTX, id string, n int) (bool, error) {
	tag, err := db.Exec(ctx, `
		UPDATE sos_alerts SET escalations = $2
		WHERE id = $1 AND acknowledged_at IS NULL AND escalations < $2`, id, n)
	if err != nil {
		return false, fmt.Errorf("SetSosEscalations: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

// DeviceHealthRow is an active tablet with its latest beacon.
type DeviceHealthRow struct {
	ID                 string
	Realm              string
	Name               string
	SiteID             string
	Health             json.RawMessage
	HealthAt           *time.Time
	LastSeenAt         *time.Time
	UnhealthyAlertedAt *time.Time
}

// ListActiveDeviceHealth returns every active tablet with a site.
func ListActiveDeviceHealth(ctx context.Context, db DBTX) ([]DeviceHealthRow, error) {
	rows, err := db.Query(ctx, `
		SELECT id, keycloak_realm, name, site_id, health, health_at, last_seen_at, unhealthy_alerted_at
		FROM devices WHERE status = 'active' AND site_id IS NOT NULL ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("ListActiveDeviceHealth: %w", err)
	}
	return collect(rows, func(r pgx.Rows) (DeviceHealthRow, error) {
		var d DeviceHealthRow
		err := r.Scan(&d.ID, &d.Realm, &d.Name, &d.SiteID, &d.Health, &d.HealthAt, &d.LastSeenAt, &d.UnhealthyAlertedAt)
		return d, err
	})
}

// MarkDeviceUnhealthyAlerted stamps the debounce time; false when another
// alert was raised within `every`.
func MarkDeviceUnhealthyAlerted(ctx context.Context, db DBTX, id string, every time.Duration) (bool, error) {
	tag, err := db.Exec(ctx, `
		UPDATE devices SET unhealthy_alerted_at = now()
		WHERE id = $1 AND (unhealthy_alerted_at IS NULL OR unhealthy_alerted_at < now() - make_interval(secs => $2))`,
		id, every.Seconds())
	if err != nil {
		return false, fmt.Errorf("MarkDeviceUnhealthyAlerted: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}
