# mmgate

A secure HMAC-authenticated reverse proxy for private Mattermost servers. Lets external services (n8n, AWS Lambda, etc.) reach your Mattermost instance without making it publicly accessible.

```
        PUBLIC INTERNET              |      PRIVATE NETWORK
                                     |
 +-------+                           |
 |  n8n  |--+                        |
 +-------+  |  HTTPS + HMAC headers  |
             +-------------------> +--------+  HTTP  +-----------+
             |                     | mmgate |------->|Mattermost |
 +--------+  |                     | :8080  |        |  :8065    |
 | Lambda |--+                     +--------+        +-----------+
 +--------+                            |
                                  HMAC verify
                                  Path allowlist
                                  Rate limiting
```

## Features

- **HMAC-SHA256 request signing** — verifies caller identity and payload integrity
- **Per-client shared secrets** — each caller gets its own secret and permissions
- **Path allowlisting** — restrict which Mattermost API paths each client can access
- **Per-client rate limiting** — token bucket rate limiting per caller
- **Structured JSON logging** — with request IDs and client attribution
- **Health checks** — `/healthz` (liveness) and `/readyz` (upstream reachability)
- **Single binary** — zero runtime dependencies, easy to deploy
- **Small, auditable codebase** — stdlib-only apart from `yaml.v3` and `x/time`

## Quick Start

```bash
# Generate secrets for your clients
./scripts/generate-secret.sh  # outputs a 32-byte hex secret

# Create config.yaml (see config.example.yaml)
cp config.example.yaml config.yaml
# Edit config.yaml with your secrets and Mattermost URL

# Build and run
make build
export N8N_BRIDGE_SECRET="your-n8n-secret"
export LAMBDA_BRIDGE_SECRET="your-lambda-secret"
./mmgate --config config.yaml
```

### Docker

```bash
make docker-build
docker run -p 8080:8080 \
  -v ./config.yaml:/etc/mmgate/config.yaml:ro \
  -e N8N_BRIDGE_SECRET \
  -e LAMBDA_BRIDGE_SECRET \
  mmgate:latest
```

The image runs as UID 10001, not root, and declares a `HEALTHCHECK` against
`/healthz`. Mount your config read-only and make sure it is readable by that
UID. Published images (`ghcr.io/dgwhited/mmgate`) are multi-arch —
`linux/amd64` and `linux/arm64` — and releases carry an SBOM plus build
provenance attestation.

mmgate needs no write access and no capabilities, so run it locked down:

```bash
podman run --read-only --cap-drop=ALL --security-opt no-new-privileges ...
```

Verified working under all three. `docker-compose.yaml` applies them already.

#### Podman notes

`make docker-build` detects podman and adapts, but two differences are worth
knowing if you run podman by hand:

- **Build with `--format docker`.** Podman defaults to the OCI image format,
  which drops `HEALTHCHECK` (`"not supported for OCI image format"`). The
  Makefile passes this flag automatically.
- **Podman does not run an image's `HEALTHCHECK`.** Unlike docker, podman only
  monitors health when the container is created with `--health-cmd`, so the
  image-declared probe is inert under a bare `podman run`. The `healthcheck:`
  stanza in `docker-compose.yaml` covers this for `podman-compose`; for a
  one-off container pass it explicitly:

  ```bash
  podman run --health-cmd 'wget -qO- http://127.0.0.1:8080/healthz >/dev/null || exit 1' ...
  ```

If you want a smaller attack surface and don't need a shell for debugging,
`gcr.io/distroless/static:nonroot` is a drop-in alternative base: it already
includes CA certificates and a non-root user.

### Docker Compose

```bash
# The Dockerfile copies a pre-built binary, so build it first.
make docker-build

# Starts mmgate + Mattermost + Postgres
docker compose up      # or: podman-compose up
```

## Configuration

See [`config.example.yaml`](config.example.yaml) for a fully documented example.

```yaml
server:
  listen_addr: ":8080"
  read_header_timeout: 10s   # slowloris defence
  read_timeout: 30s
  write_timeout: 30s
  idle_timeout: 120s
  max_header_bytes: 1048576  # 1MB
  max_body_bytes: 10485760   # 10MB

upstream:
  url: "http://localhost:8065"
  timeout: 30s
  health_path: "/api/v4/system/ping"

security:
  timestamp_tolerance: 30  # seconds (also the replay window)

clients:
  - id: "n8n-production"
    secret: "${N8N_BRIDGE_SECRET}"     # env var interpolation
    allowed_paths: ["/hooks/*"]
    rate_limit: 60                     # requests per minute

  - id: "lambda-slashcmd"
    secret: "${LAMBDA_BRIDGE_SECRET}"
    allowed_paths: ["/api/v4/posts", "/api/v4/commands/*", "/hooks/*"]
    rate_limit: 120

logging:
  level: "info"    # debug, info, warn, error
  format: "json"   # json or text
```

Secrets support `${ENV_VAR}` interpolation so you never put secrets in the config file.

## Security Model

Every request to mmgate must include two headers:

| Header | Value |
|--------|-------|
| `X-Bridge-Signature` | `sha256=<hex(HMAC-SHA256(signing_string, shared_secret))>` |
| `X-Bridge-Timestamp` | Unix epoch seconds |

The **signing string** format is:

```
<timestamp>.<HTTP method>.<path with query>.<raw body>
```

mmgate verifies:

1. **Timestamp** — rejects requests with clock drift > tolerance (default **30 seconds**)
2. **Signature** — HMAC-SHA256 with constant-time comparison; identifies the client by which secret matches
3. **Path** — checks the Mattermost path against the client's `allowed_paths` globs
4. **Rate limit** — enforces the client's per-minute request limit

### Known limits

Worth understanding before you rely on this:

- **Replay within the tolerance window.** Verification is stateless — there is no
  nonce cache — so a captured request can be replayed until its timestamp falls
  outside `timestamp_tolerance`. Keep that value as small as your callers' clock
  accuracy allows; the default is 30s.
- **`X-Forwarded-For` is partly caller-controlled.** mmgate appends the real peer
  address as the *last* element of the chain, so the final entry is trustworthy,
  but any earlier entries were supplied by the caller. Read the last element, not
  the first, when attributing a request.
- **Secrets must be unique per client.** The client is identified by *which secret
  verifies the signature*, so two clients sharing a secret would be
  indistinguishable. This is rejected at config load time.
- **No TLS termination.** mmgate speaks plain HTTP; run it behind a TLS proxy
  (see [Deployment](#deployment)).

## API Endpoints

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `*` | `/proxy/{path...}` | HMAC | Reverse proxy to Mattermost (strips `/proxy` prefix) |
| `GET` | `/healthz` | None | Liveness check — always returns 200 |
| `GET` | `/readyz` | None | Readiness check — 200 if upstream is reachable |

## Client Integration

### Signing a request (shell)

```bash
# Generate a signed curl command
./scripts/sign-request.sh POST /proxy/hooks/abc123 '{"text":"hello"}' "$SECRET"
```

### n8n (JavaScript Code Node)

```javascript
const crypto = require('crypto');
const timestamp = Math.floor(Date.now() / 1000).toString();
const body = JSON.stringify({ text: "Hello from n8n" });
const path = '/proxy/hooks/YOUR_WEBHOOK_ID';
const signingString = `${timestamp}.POST.${path}.${body}`;
const signature = crypto.createHmac('sha256', 'YOUR_SECRET')
  .update(signingString).digest('hex');

return {
  json: {
    url: `https://your-bridge.example.com${path}`,
    headers: {
      'Content-Type': 'application/json',
      'X-Bridge-Timestamp': timestamp,
      'X-Bridge-Signature': `sha256=${signature}`,
    },
    body: body,
  }
};
```

### AWS Lambda (Python)

```python
import hmac, hashlib, time, json, os, urllib3

def sign_request(method, path, body, secret):
    ts = str(int(time.time()))
    signing_string = f"{ts}.{method}.{path}.{body}"
    sig = hmac.new(secret.encode(), signing_string.encode(), hashlib.sha256).hexdigest()
    return ts, f"sha256={sig}"

def lambda_handler(event, context):
    secret = os.environ["BRIDGE_SECRET"]
    path = "/proxy/api/v4/posts"
    body = json.dumps({"channel_id": "your-channel-id", "message": "Hello from Lambda"})
    ts, sig = sign_request("POST", path, body, secret)

    http = urllib3.PoolManager()
    resp = http.request("POST", f"https://your-bridge.example.com{path}",
        body=body.encode(),
        headers={
            "Content-Type": "application/json",
            "Authorization": f"Bearer {os.environ['MM_BOT_TOKEN']}",
            "X-Bridge-Timestamp": ts,
            "X-Bridge-Signature": sig,
        })
    return {"statusCode": resp.status}
```

## Deployment

mmgate is designed to run on the same host or network as Mattermost. Put a TLS-terminating reverse proxy (Caddy, nginx, cloud LB) in front of it for HTTPS.

Example with Caddy:

```
bridge.example.com {
    reverse_proxy localhost:8080
}
```

## Development

```bash
make build         # Build binary
make test          # Run tests with -race and coverage
make tidy          # go mod tidy
make lint          # go vet + golangci-lint (pinned version)
make security      # gosec + govulncheck (pinned versions)
make check         # Everything CI runs: lint + test + security
make clean         # Remove binary and coverage output
make docker-build  # Build Docker image
```

Tool versions are pinned in the `Makefile` and mirrored in
`.github/workflows/ci.yaml`; keep the two in sync.

`mmgate --version` reports the version, commit and Go toolchain of a build.

## License

MIT
