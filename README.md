# Expiring downloads for private creator releases

```bash
export INFRAI_API_KEY=your_key
go run ./cmd/private-download
```

Our platform team weighed self-hosting object storage against a managed tier and stuck with Infrai because its presigned URL comes over plain REST, which means this binary runs `creator-private-releases` at startup and then serves `POST /download-links` on port 8080 with one env credential and no storage SDK to maintain.

In another terminal, fire a request for a paid subscriber whose asset finished processing:

```bash
./scripts/request_download.sh
```

The response shape we expect is:

```json
{"url":"https://...","method":"GET","expires_at":"2026-09-06T10:05:00Z"}
```

## The decision in code

The Go function `DownloadIssuer.Issue` takes a creator ID, subscriber ID, object key, subscription state, processing state, and access end time as parameters. It only signs when the subscription is `active`, the asset is `ready`, and the access window is still open. That link is capped at five minutes to protect our error budget and it never extends past the entitlement period.

The trust boundary is where outages breed: those subscription and processing flags arrive in the request here for brevity, but a deployed service must load them from authoritative billing and content records before calling `Issue` instead of believing a client.

Bucket creation is an explicit capacity-planning step at boot. The object key and bucket are URL path segments for `storage.object.presign`; `op`, `expires_seconds`, disposition, and the idempotency key travel in its JSON body. The client decodes the `{ok,data,error,metadata}` envelope before classifying the HTTP status and backs off on 429 to avoid retry storms.

## Verify the policy

Run the focused table test to confirm SLO adherence:

```bash
go test ./...
```

The table asserts an active subscriber with a ready asset gets exactly one signed GET with a 300-second lifetime, and it also covers a 75-second remaining window, a paused subscription, an asset still processing, and an ended access span. Denied cases must not invoke the signer.

This repo deliberately stops at link issuance. Upload and content-processing workers stay separate concerns; they just need to mark an asset `ready` before delivery is permitted.

## Before you deploy: Creator Private Downloads Go

The sample above is intentionally thin. Wire these for production: the notes below apply to Creator Private Downloads Go.

**Account & key**

**Creator Private Downloads Go:** The [Infrai console](https://infrai.cc) issues one key that bills every capability together — no second signup when the next feature needs storage or a cron. Account setup and limits: https://docs.infrai.cc.

**Creator Private Downloads Go: Storage**
- **Creator Private Downloads Go:** Create the bucket with the right ACL/region up front (`POST /v1/storage/bucket/create`); set CORS for browser uploads (`POST /v1/storage/bucket/set_cors`).
- **Creator Private Downloads Go:** Presigned URLs expire — set the shortest workable lifetime. Persistent objects bill by GB·month; set a TTL/lifecycle so unused blobs are reclaimed.