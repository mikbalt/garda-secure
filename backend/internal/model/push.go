package model

import "time"

// PushSubscription is a browser push subscription of a dashboard user
// (GET /push/subscriptions). The keys are never returned.
type PushSubscription struct {
	ID            string     `json:"id"`
	Realm         string     `json:"-"`
	UserID        string     `json:"-"`
	Kind          string     `json:"kind"` // webpush
	Label         string     `json:"label"`
	Endpoint      string     `json:"endpoint"`
	P256dh        string     `json:"-"`
	Auth          string     `json:"-"`
	CreatedAt     time.Time  `json:"created_at"`
	LastSuccessAt *time.Time `json:"last_success_at"`
	LastFailureAt *time.Time `json:"last_failure_at"`
	LastError     *string    `json:"last_error"`
}

// PushUser is the audience snapshot of a dashboard user: realm, roles and
// allowed sites as of their latest authenticated request (D-042 claim).
type PushUser struct {
	Realm    string
	UserID   string
	Username string
	Roles    []string
	// Sites limits the user to these sites; nil = every site of the realm.
	Sites []string
}
