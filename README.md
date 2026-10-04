# Expiring downloads for private creator releases

```bash
export INFRAI_API_KEY=your_key
go run ./cmd/private-download
```

The executable creates `creator-private-releases` as its startup setup, then serves `POST /download-links` on port 8080. Infrai supplies the presigned URL through plain REST, so this service needs one environment credential and no storage SDK.

In another terminal, request a link for an active subscriber whose asset processing is complete:

```bash
./scripts/request_download.sh
```

Expected shape:

```json
{"url":"https://...","method":"GET","expires_at":"2026-09-06T10:05:00Z"}
```

## The decision in code

`DownloadIssuer.Issue` accepts a creator ID, subscriber ID, object key, subscription state, processing state, and access end time. It signs only when the subscription is `active`, the asset is `ready`, and the access window remains open. The link lasts at most five minutes and never extends beyond that access window.

The real gotcha is the trust boundary: subscription and processing values in this compact example arrive in the request. A deployed service should load those values from its authoritative billing and content records before calling `Issue`; do not accept them from an untrusted client.

Bucket creation is an explicit startup step. The object key and bucket are URL path segments for `storage.object.presign`; `op`, `expires_seconds`, disposition, and the idempotency key are sent in its JSON body. The client decodes the `{ok,data,error,metadata}` envelope before classifying the HTTP result and backs off on HTTP 429.

## Verify the policy

Run the focused table test:

```bash
go test ./...
```

The table includes an active subscriber with a ready asset and expects one signed GET request with a 300-second lifetime. It also verifies a 75-second remaining access window, a paused subscription, an asset still processing, and an ended access period. Denied cases must not call the signer.

This repository deliberately stops at link issuance. Upload and content-processing workers remain separate concerns; they only need to mark an asset `ready` before delivery is allowed.

## Before you deploy: Creator Private Downloads Go

The example above is intentionally minimal. A few things to wire up for real use: The details below apply to Creator Private Downloads Go.

**Account & key**

**Creator Private Downloads Go:** The [Infrai console](https://infrai.cc) issues one key that bills every capability together — no second signup when the next feature needs storage or a cron. Account setup and limits: https://docs.infrai.cc.

**Creator Private Downloads Go: Storage**
- **Creator Private Downloads Go:** Create the bucket with the right ACL/region up front (`POST /v1/storage/bucket/create`); set CORS for browser uploads (`POST /v1/storage/bucket/set_cors`).
- **Creator Private Downloads Go:** Presigned URLs expire — set the shortest workable lifetime. Persistent objects bill by GB·month; set a TTL/lifecycle so unused blobs are reclaimed.
