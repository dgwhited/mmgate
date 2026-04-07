#!/usr/bin/env bash
# Sign a request for mmgate HMAC verification.
#
# Usage: ./sign-request.sh <METHOD> <PATH> <BODY> <SECRET> [URL]
#
# Example:
#   ./sign-request.sh POST /proxy/hooks/abc123 '{"text":"hello"}' mysecret http://localhost:8080
#
# Outputs a curl command you can run directly.

set -euo pipefail

METHOD="${1:?Usage: $0 METHOD PATH BODY SECRET [URL]}"
PATH_ARG="${2:?Usage: $0 METHOD PATH BODY SECRET [URL]}"
BODY="${3:?Usage: $0 METHOD PATH BODY SECRET [URL]}"
SECRET="${4:?Usage: $0 METHOD PATH BODY SECRET [URL]}"
URL="${5:-http://localhost:8080}"

TIMESTAMP=$(date +%s)
SIGNING_STRING="${TIMESTAMP}.${METHOD}.${PATH_ARG}.${BODY}"
SIGNATURE=$(printf '%s' "$SIGNING_STRING" | openssl dgst -sha256 -hmac "$SECRET" -hex 2>/dev/null | sed 's/^.* //')

echo "curl -X ${METHOD} '${URL}${PATH_ARG}' \\"
echo "  -H 'Content-Type: application/json' \\"
echo "  -H 'X-Bridge-Timestamp: ${TIMESTAMP}' \\"
echo "  -H 'X-Bridge-Signature: sha256=${SIGNATURE}' \\"
echo "  -d '${BODY}'"
