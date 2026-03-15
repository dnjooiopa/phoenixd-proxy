# phoenixd-proxy

A simple webhook proxy service that receives [phoenixd](https://phoenix.acinq.co/server) payment webhook requests and forwards them to multiple registered target endpoints.

## Prerequisites

- Go 1.25+
- GCC (required by go-sqlite3)

## Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `ENVIRONMENT` | No | - | Set to `production` to enable release mode and use `/app/data/proxy.db` as the database path |
| `API_KEY` | Yes | - | API key for authenticating endpoint management requests |
| `ADDRESS` | No | - | Server bind address (e.g. `127.0.0.1`, `0.0.0.0`) |
| `PORT` | No | `8080` | Server listening port |

## Docker installation

```sh
docker run --name phoenixd-proxy \
  -dp 9780:8080 \
  -v /path/to/data/:/app/data/ \
  -e ENVIRONMENT=production \
  -e API_KEY=my-secret-key \
  --network lnf \
  --restart unless-stopped \
  ghcr.io/dnjooiopa/phoenixd-proxy:latest
```

## Running

```bash
API_KEY=my-secret-key go run .
```

## API Reference

### Endpoints Management

All endpoint management routes require the `X-API-KEY` header.

#### List Endpoints

```bash
curl localhost:8080/endpoints \
  -H 'X-API-KEY: my-secret-key'
```

Response: `200 OK`
```json
[
  {
    "id": 1,
    "url": "https://myapp.com/payment-hook",
    "created_at": "2026-03-15 10:00:00"
  }
]
```

#### Register Endpoint

```bash
curl -X POST localhost:8080/endpoints \
  -H 'Content-Type: application/json' \
  -H 'X-API-KEY: my-secret-key' \
  -d '{"url": "https://myapp.com/payment-hook"}'
```

Response: `201 Created`
```json
{
  "id": 1,
  "url": "https://myapp.com/payment-hook",
  "created_at": "2026-03-15 10:00:00"
}
```

Returns `409 Conflict` if the URL is already registered.

#### Delete Endpoint

```bash
curl -X DELETE localhost:8080/endpoints/1 \
  -H 'X-API-KEY: my-secret-key'
```

Response: `204 No Content`

Returns `404 Not Found` if the endpoint ID does not exist.

### Webhook Receiver

The webhook endpoint does not require authentication. It receives the payload and forwards it to all registered endpoints asynchronously.

```bash
curl -X POST localhost:8080/webhook \
  -H 'Content-Type: application/json' \
  -H 'X-Phoenix-Signature: <signature>' \
  -d '{
    "type": "payment_received",
    "timestamp": 1748269006918,
    "amountSat": 1,
    "paymentHash": "7db610f2...",
    "externalId": "",
    "payerNote": "",
    "payerKey": ""
  }'
```

Response: `200 OK`
```json
{
  "status": "ok",
  "forwarded_to": 2
}
```

The `Content-Type` and `X-Phoenix-Signature` headers are forwarded to all registered endpoints.

## Testing

```bash
go test -v ./...
```

## How It Works

1. Register target endpoints via the management API (protected by API key).
2. Configure phoenixd to send webhooks to `http://<host>:8080/webhook`.
3. When a webhook arrives, the proxy reads the raw body and forwards it to all registered endpoints concurrently.
4. The proxy responds immediately with `200 OK` — forwarding happens asynchronously.
5. Failed forwards are logged to stdout but do not affect the webhook response.
