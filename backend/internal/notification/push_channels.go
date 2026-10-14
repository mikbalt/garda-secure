package notification

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/securepatrol/backend/internal/push"
	"github.com/securepatrol/backend/internal/repository"
	"github.com/securepatrol/backend/internal/settings"
)

// Push delivery channels. Unlike e-mail and WhatsApp they are not in the
// rule registry (Channels): nobody types a recipient, users subscribe their
// browsers and tablets register an FCM token.

// IsPushChannel reports whether a delivery channel is a push transport.
func IsPushChannel(ch string) bool {
	return ch == repository.ChannelWebPush || ch == repository.ChannelFCM
}

// webPushChannel sends a staged browser push. recipient is the
// subscription ID, body the JSON payload.
type webPushChannel struct {
	web *push.WebPush
	db  repository.DBTX
}

func (c webPushChannel) Send(ctx context.Context, _ settings.Channels, recipient, _, body string) error {
	sub, err := repository.GetPushSubscription(ctx, c.db, recipient)
	if errors.Is(err, repository.ErrNotFound) {
		return fmt.Errorf("%w: subscription was removed", push.ErrGone)
	}
	if err != nil {
		return err
	}
	return SendWebPush(ctx, c.db, c.web, sub.ID, push.Subscription{Endpoint: sub.Endpoint, P256dh: sub.P256dh, Auth: sub.Auth}, []byte(body))
}

// SendWebPush sends one payload to a subscription and keeps its bookkeeping:
// success/failure stamps, and removal when the push service says it is gone.
func SendWebPush(ctx context.Context, db repository.DBTX, web *push.WebPush, id string, sub push.Subscription, payload []byte) error {
	var p push.WebPayload
	_ = json.Unmarshal(payload, &p)
	err := web.Send(ctx, sub, payload, push.WebOptionsFor(p))
	switch {
	case err == nil:
		if merr := repository.MarkPushSubscription(ctx, db, id, nil); merr != nil {
			return nil // sent; bookkeeping is best effort
		}
	case errors.Is(err, push.ErrGone):
		_ = repository.DeletePushSubscription(ctx, db, id)
	default:
		msg := truncate(err.Error(), 300)
		_ = repository.MarkPushSubscription(ctx, db, id, &msg)
	}
	return err
}

// fcmChannel sends a staged tablet data message. recipient is the FCM token,
// body the JSON TabletMessage.
type fcmChannel struct {
	fcm *push.FCM
	db  repository.DBTX
}

func (c fcmChannel) Send(ctx context.Context, _ settings.Channels, recipient, _, body string) error {
	var m push.TabletMessage
	if err := json.Unmarshal([]byte(body), &m); err != nil {
		return fmt.Errorf("%w: invalid tablet message", push.ErrGone)
	}
	err := c.fcm.Send(ctx, recipient, m)
	if errors.Is(err, push.ErrGone) {
		_ = repository.ClearPushToken(ctx, c.db, recipient)
	}
	return err
}

// UsePush adds the push channels to the dispatcher (no-op for a disabled
// transport: its queued rows then fail with "unsupported channel").
func (w *Worker) UsePush(s *push.Service) {
	if s.WebEnabled() {
		w.channels[repository.ChannelWebPush] = webPushChannel{web: s.Web, db: w.db}
	}
	if s.FCMEnabled() {
		w.channels[repository.ChannelFCM] = fcmChannel{fcm: s.FCM, db: w.db}
	}
}
