#!/bin/sh
# Creates the signer's Ed25519 key on its own volume if there is none (D-071).
# Run before the signer starts (make up does it). The key never leaves the
# volume; the fingerprint is printed so it can be pinned out of band.
set -eu
cd "$(dirname "$0")/.."
if fp=$(docker compose run --rm --no-deps -T signer pubkey 2>/dev/null); then
  echo "signer key present, fingerprint: $fp"
  exit 0
fi
docker compose run --rm --no-deps -T signer keygen
echo "signer-key: pin the fingerprint above out of band (runbook: Signer service)"
