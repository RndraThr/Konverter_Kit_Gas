package notifications

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

const (
	fcmScope   = "https://www.googleapis.com/auth/firebase.messaging"
	fcmChannel = "konkit_finance"
	// fcmQuietChannel: low importance, no sound (made by the app 1.3.4+;
	// older apps show it on their default channel).
	fcmQuietChannel = "konkit_quiet"
)

// FCMSender sends through the Firebase Cloud Messaging HTTP v1 API with a
// service account key.
type FCMSender struct {
	endpoint string
	client   *http.Client
}

// NewFCMSender reads the service account key made in the Firebase console.
func NewFCMSender(ctx context.Context, serviceAccountJSON []byte) (*FCMSender, error) {
	var key struct {
		ProjectID string `json:"project_id"`
	}
	if err := json.Unmarshal(serviceAccountJSON, &key); err != nil || key.ProjectID == "" {
		return nil, errors.New("FCM service account key has no project_id")
	}
	credentials, err := google.CredentialsFromJSON(ctx, serviceAccountJSON, fcmScope)
	if err != nil {
		return nil, fmt.Errorf("read FCM service account key: %w", err)
	}
	client := oauth2.NewClient(context.WithoutCancel(ctx), credentials.TokenSource)
	client.Timeout = 15 * time.Second
	return &FCMSender{endpoint: "https://fcm.googleapis.com/v1/projects/" + key.ProjectID + "/messages:send", client: client}, nil
}

func (s *FCMSender) Send(ctx context.Context, push Push) error {
	body, err := fcmRequest(push)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, s.endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := s.client.Do(request)
	if err != nil {
		return fmt.Errorf("send FCM message: %w", err)
	}
	defer response.Body.Close()
	detail, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
	if response.StatusCode == http.StatusOK {
		return nil
	}
	if fcmTokenGone(response.StatusCode, detail) {
		return ErrTokenGone
	}
	return fmt.Errorf("FCM status %d: %s", response.StatusCode, detail)
}

// fcmRequest builds the message: high priority wakes a sleeping phone, and
// the notification block is shown by Android while the app is closed. A tag
// replaces the earlier notification of the same tag; a silent one goes to
// the quiet channel without a sound.
func fcmRequest(push Push) ([]byte, error) {
	channel, priority := fcmChannel, "PRIORITY_HIGH"
	if push.Silent {
		channel, priority = fcmQuietChannel, "PRIORITY_LOW"
	}
	notification := map[string]any{
		"channel_id":              channel,
		"notification_priority":   priority,
		"visibility":              "PRIVATE",
		"default_sound":           !push.Silent,
		"default_vibrate_timings": !push.Silent,
	}
	data := map[string]string{"link": push.Link, "notification_id": push.ID}
	if push.Tag != "" {
		notification["tag"] = push.Tag
		data["tag"] = push.Tag
	}
	if push.Silent {
		data["silent"] = "1"
	}
	return json.Marshal(map[string]any{"message": map[string]any{
		"token":        push.Token,
		"notification": map[string]string{"title": push.Title, "body": push.Body},
		"data":         data,
		"android":      map[string]any{"priority": "HIGH", "notification": notification},
	}})
}

func fcmTokenGone(status int, body []byte) bool {
	return status == http.StatusNotFound || (status == http.StatusBadRequest && strings.Contains(string(body), "UNREGISTERED"))
}
