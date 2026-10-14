package push

import (
	"bytes"
	"context"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/securepatrol/backend/internal/netguard"
)

// Google endpoints of the FCM HTTP v1 API and its OAuth2 token exchange.
const (
	googleTokenURL = "https://oauth2.googleapis.com/token" // #nosec G101 -- an endpoint URL, not a credential
	fcmSendBase    = "https://fcm.googleapis.com/v1/projects/"
	fcmScope       = "https://www.googleapis.com/auth/firebase.messaging"
)

// Tablet message types (FCM data message field "type").
const (
	TabletSOS              = "sos"
	TabletSOSAck           = "sos_ack"
	TabletPatrolNotStarted = "patrol_not_started"
	TabletDeviceStatus     = "device_status"
	TabletSync             = "sync"
)

// TabletMessage is the FCM data message a tablet receives. Every field is a
// string (FCM data values must be strings).
type TabletMessage struct {
	Type   string `json:"type"`
	SiteID string `json:"site_id"`
	RefID  string `json:"ref_id"`
	Title  string `json:"title"`
	Body   string `json:"body"`
}

// TabletTTL is how long FCM keeps an undelivered message: an SOS is only
// useful for a minute, a status change for a day.
func TabletTTL(msgType string) time.Duration {
	switch msgType {
	case TabletSOS:
		return time.Minute
	case TabletSOSAck:
		return 5 * time.Minute
	case TabletPatrolNotStarted:
		return 15 * time.Minute
	case TabletDeviceStatus:
		return 24 * time.Hour
	default:
		return time.Hour
	}
}

// serviceAccount is the part of a Google service-account key file we use.
type serviceAccount struct {
	Type         string `json:"type"`
	ProjectID    string `json:"project_id"`
	PrivateKeyID string `json:"private_key_id"`
	PrivateKey   string `json:"private_key"`
	ClientEmail  string `json:"client_email"`
	TokenURI     string `json:"token_uri"`
}

// FCM sends data messages through the FCM HTTP v1 API, authenticated with a
// service-account OAuth2 token (JWT bearer grant, cached until shortly
// before it expires).
type FCM struct {
	projectID, clientEmail, keyID string
	key                           *rsa.PrivateKey
	tokenURL, sendURL             string
	client                        *http.Client

	mu       sync.Mutex
	token    string
	tokenExp time.Time
}

// LoadFCM reads the service-account file. A path that is empty, missing or
// an empty file (the compose placeholder) disables FCM: (nil, nil).
func LoadFCM(path string) (*FCM, error) {
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path) // #nosec G304 G703 -- operator-configured secret file
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("FCM_CREDENTIALS_FILE: %w", err)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, nil
	}
	return ParseFCMCredentials(data)
}

// ParseFCMCredentials builds the FCM client from service-account JSON.
func ParseFCMCredentials(data []byte) (*FCM, error) {
	var sa serviceAccount
	if err := json.Unmarshal(data, &sa); err != nil {
		return nil, errors.New("FCM credentials are not valid JSON")
	}
	if sa.Type != "service_account" || sa.ProjectID == "" || sa.ClientEmail == "" || sa.PrivateKey == "" {
		return nil, errors.New("FCM credentials must be a service-account key (type, project_id, client_email, private_key)")
	}
	if sa.TokenURI != "" && sa.TokenURI != googleTokenURL {
		return nil, fmt.Errorf("FCM credentials token_uri must be %s", googleTokenURL)
	}
	for _, r := range sa.ProjectID {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
			return nil, errors.New("FCM credentials project_id is not a Firebase project id")
		}
	}
	key, err := jwt.ParseRSAPrivateKeyFromPEM([]byte(sa.PrivateKey))
	if err != nil {
		return nil, errors.New("FCM credentials private_key is not an RSA PEM key")
	}
	return &FCM{
		projectID: sa.ProjectID, clientEmail: sa.ClientEmail, keyID: sa.PrivateKeyID, key: key,
		tokenURL: googleTokenURL, sendURL: fcmSendBase + sa.ProjectID + "/messages:send",
		client: guardedClient(netguard.Policy{}),
	}, nil
}

// ProjectID is the Firebase project messages are sent through.
func (f *FCM) ProjectID() string { return f.projectID }

// accessToken returns a cached OAuth2 token or exchanges a new JWT for one.
func (f *FCM) accessToken(ctx context.Context) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.token != "" && time.Until(f.tokenExp) > 5*time.Minute {
		return f.token, nil
	}
	now := time.Now()
	t := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"iss": f.clientEmail, "scope": fcmScope, "aud": f.tokenURL,
		"iat": now.Unix(), "exp": now.Add(time.Hour).Unix(),
	})
	if f.keyID != "" {
		t.Header["kid"] = f.keyID
	}
	assertion, err := t.SignedString(f.key)
	if err != nil {
		return "", fmt.Errorf("fcm token: sign: %w", err)
	}
	form := url.Values{"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"}, "assertion": {assertion}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, f.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := f.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fcm token: %w", redactURL(err))
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<10))
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fcm token: %w", &StatusError{Status: resp.StatusCode, Detail: excerpt(body)})
	}
	var tok struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &tok); err != nil || tok.AccessToken == "" {
		return "", errors.New("fcm token: response has no access_token")
	}
	if tok.ExpiresIn <= 0 {
		tok.ExpiresIn = 3600
	}
	f.token, f.tokenExp = tok.AccessToken, now.Add(time.Duration(tok.ExpiresIn)*time.Second)
	return f.token, nil
}

func (f *FCM) dropToken() {
	f.mu.Lock()
	f.token = ""
	f.mu.Unlock()
}

// fcmRequest is the HTTP v1 message body.
type fcmRequest struct {
	Message fcmMessage `json:"message"`
}

type fcmMessage struct {
	Token   string            `json:"token"`
	Data    map[string]string `json:"data"`
	Android fcmAndroid        `json:"android"`
	APNS    fcmAPNS           `json:"apns"`
}

type fcmAndroid struct {
	Priority    string `json:"priority"`
	TTL         string `json:"ttl"`
	CollapseKey string `json:"collapse_key"`
}

type fcmAPNS struct {
	Headers map[string]string `json:"headers"`
	Payload map[string]any    `json:"payload"`
}

// BuildFCMRequest is the high-priority data message for one tablet token:
// Android priority high with a per-type TTL and collapse key; on iOS a
// background (content-available) push.
func BuildFCMRequest(token string, m TabletMessage, now time.Time) any {
	ttl := TabletTTL(m.Type)
	return fcmRequest{Message: fcmMessage{
		Token: token,
		Data: map[string]string{
			"type": m.Type, "site_id": m.SiteID, "ref_id": m.RefID, "title": m.Title, "body": m.Body,
		},
		Android: fcmAndroid{Priority: "high", TTL: strconv.Itoa(int(ttl/time.Second)) + "s", CollapseKey: m.Type},
		APNS: fcmAPNS{
			Headers: map[string]string{
				"apns-priority": "5", "apns-push-type": "background", "apns-collapse-id": m.Type,
				"apns-expiration": strconv.FormatInt(now.Add(ttl).Unix(), 10),
			},
			Payload: map[string]any{"aps": map[string]any{"content-available": 1}},
		},
	}}
}

// Send delivers one data message. An unregistered token wraps ErrGone.
func (f *FCM) Send(ctx context.Context, token string, m TabletMessage) error {
	if f == nil {
		return ErrDisabled
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, sendTimeout)
		defer cancel()
	}
	access, err := f.accessToken(ctx)
	if err != nil {
		return err
	}
	b, err := json.Marshal(BuildFCMRequest(token, m, time.Now()))
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, f.sendURL, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+access)
	req.Header.Set("Content-Type", "application/json")
	resp, err := f.client.Do(req)
	if err != nil {
		return fmt.Errorf("fcm send: %w", redactURL(err))
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<10))
	if resp.StatusCode == http.StatusOK {
		return nil
	}
	se := &StatusError{Status: resp.StatusCode, Detail: excerpt(body)}
	switch {
	case resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone || fcmErrorCode(body) == "UNREGISTERED":
		return fmt.Errorf("%w: %w", ErrGone, se)
	case resp.StatusCode == http.StatusUnauthorized:
		f.dropToken() // expired or revoked: exchange a new one on the retry
	}
	return se
}

// fcmErrorCode extracts details[].errorCode of an FCM error response.
func fcmErrorCode(body []byte) string {
	var e struct {
		Error struct {
			Details []struct {
				ErrorCode string `json:"errorCode"`
			} `json:"details"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &e) != nil {
		return ""
	}
	for _, d := range e.Error.Details {
		if d.ErrorCode != "" {
			return d.ErrorCode
		}
	}
	return ""
}

func excerpt(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 300 {
		s = s[:300]
	}
	return s
}
