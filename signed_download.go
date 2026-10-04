package delivery

import (
	"context"
	"errors"
	"fmt"
	"time"
)

const MaxLinkLifetime = 5 * time.Minute

var ErrDownloadDenied = errors.New("download is not available")

type DownloadRequest struct {
	CreatorID         string    `json:"creator_id"`
	SubscriberID      string    `json:"subscriber_id"`
	AssetKey          string    `json:"asset_key"`
	SubscriptionState string    `json:"subscription_state"`
	ProcessingState   string    `json:"processing_state"`
	AccessEndsAt      time.Time `json:"access_ends_at"`
}

type DownloadLink struct {
	URL       string    `json:"url"`
	Method    string    `json:"method"`
	ExpiresAt time.Time `json:"expires_at"`
}

type ObjectSigner interface {
	PresignObject(context.Context, string, string, PresignInput) (PresignResult, error)
}

type DownloadIssuer struct {
	Bucket string
	Signer ObjectSigner
	Now    func() time.Time
}

func (d DownloadIssuer) Issue(ctx context.Context, request DownloadRequest) (DownloadLink, error) {
	now := d.Now()
	if request.CreatorID == "" || request.SubscriberID == "" || request.AssetKey == "" {
		return DownloadLink{}, fmt.Errorf("%w: missing identity or asset", ErrDownloadDenied)
	}
	if request.SubscriptionState != "active" || request.ProcessingState != "ready" || !request.AccessEndsAt.After(now) {
		return DownloadLink{}, ErrDownloadDenied
	}

	lifetime := min(MaxLinkLifetime, request.AccessEndsAt.Sub(now))
	expiresSeconds := max(1, int(lifetime/time.Second))
	result, err := d.Signer.PresignObject(ctx, d.Bucket, request.AssetKey, PresignInput{
		Operation:           "get",
		ExpiresSeconds:      expiresSeconds,
		ResponseDisposition: `attachment; filename="` + request.AssetKey + `"`,
		IdempotencyKey:      request.CreatorID + ":" + request.SubscriberID + ":" + request.AssetKey,
	})
	if err != nil {
		return DownloadLink{}, err
	}
	return DownloadLink{URL: result.URL, Method: result.Method, ExpiresAt: now.Add(time.Duration(expiresSeconds) * time.Second)}, nil
}
