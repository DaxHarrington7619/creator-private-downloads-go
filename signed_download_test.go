package delivery

import (
	"context"
	"errors"
	"testing"
	"time"
)

type recordingSigner struct {
	input PresignInput
	calls int
}

func (s *recordingSigner) PresignObject(_ context.Context, _, _ string, input PresignInput) (PresignResult, error) {
	s.calls++
	s.input = input
	return PresignResult{URL: "https://download.example/signed", Method: "GET"}, nil
}

func TestDownloadIssuerDecision(t *testing.T) {
	now := time.Date(2026, time.September, 6, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name             string
		subscription     string
		processing       string
		accessEndsAt     time.Time
		wantAllowed      bool
		wantExpirySecond int
	}{
		{name: "ready asset for active subscriber", subscription: "active", processing: "ready", accessEndsAt: now.Add(time.Hour), wantAllowed: true, wantExpirySecond: 300},
		{name: "access window limits link", subscription: "active", processing: "ready", accessEndsAt: now.Add(75 * time.Second), wantAllowed: true, wantExpirySecond: 75},
		{name: "paused subscriber", subscription: "paused", processing: "ready", accessEndsAt: now.Add(time.Hour)},
		{name: "asset still processing", subscription: "active", processing: "processing", accessEndsAt: now.Add(time.Hour)},
		{name: "access period ended", subscription: "active", processing: "ready", accessEndsAt: now},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			signer := &recordingSigner{}
			issuer := DownloadIssuer{Bucket: "creator-releases", Signer: signer, Now: func() time.Time { return now }}
			link, err := issuer.Issue(context.Background(), DownloadRequest{
				CreatorID: "creator-42", SubscriberID: "subscriber-9", AssetKey: "releases/ledger-template.pdf",
				SubscriptionState: test.subscription, ProcessingState: test.processing, AccessEndsAt: test.accessEndsAt,
			})
			if test.wantAllowed {
				if err != nil {
					t.Fatalf("Issue() error = %v", err)
				}
				if signer.calls != 1 || signer.input.ExpiresSeconds != test.wantExpirySecond {
					t.Fatalf("signer calls = %d, expiry = %d", signer.calls, signer.input.ExpiresSeconds)
				}
				if link.Method != "GET" {
					t.Fatalf("method = %q", link.Method)
				}
				return
			}
			if !errors.Is(err, ErrDownloadDenied) || signer.calls != 0 {
				t.Fatalf("error = %v, signer calls = %d", err, signer.calls)
			}
		})
	}
}
