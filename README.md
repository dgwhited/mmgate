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
- **~500 lines of Go** — minimal, auditable codebase

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

### Docker Compose

```bash
# Starts mmgate + Mattermost + Postgres
docker compose up
```

## Configuration

See [`config.example.yaml`](config.example.yaml) for a fully documented example.

```yaml
server:
  listen_addr: ":8080"
  read_timeout: 30s
  write_timeout: 30s
  max_body_bytes: 10485760  # 10MB

upstream:
  url: "http://localhost:8065"
  timeout: 30s
  health_path: "/api/v4/system/ping"

security:
  timestamp_tolerance: 300  # seconds

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

1. **Timestamp** — rejects requests with clock drift > tolerance (default 5 minutes)
2. **Signature** — HMAC-SHA256 with constant-time comparison; identifies the client by which secret matches
3. **Path** — checks the Mattermost path against the client's `allowed_paths` globs
4. **Rate limit** — enforces the client's per-minute request limit

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
make build       # Build binary
make test        # Run tests
make lint        # Run go vet
make clean       # Remove binary
make docker-build  # Build Docker image
```

## License

MIT
