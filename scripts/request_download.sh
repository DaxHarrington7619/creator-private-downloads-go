#!/bin/sh
set -eu

curl --fail-with-body --request POST http://localhost:8080/download-links \
  --header 'Content-Type: application/json' \
  --data '{"creator_id":"creator-42","subscriber_id":"subscriber-9","asset_key":"releases/ledger-template.pdf","subscription_state":"active","processing_state":"ready","access_ends_at":"2030-01-01T00:00:00Z"}'
