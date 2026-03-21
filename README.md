# phoenixd-proxy

A simple webhook proxy service that receives [phoenixd](https://phoenix.acinq.co/server) payment webhook requests and forwards them to multiple registered target endpoints.

## Installation

```sh
docker run --name phoenixd-proxy \
  -dp 9780:8080 \
  -v /path/to/data/:/app/data/ \
  -e ENVIRONMENT=production \
  -e PHOENIXD_URL=http://phoenixd:9740 \
  -e PHOENIXD_PASSWORD=my-phoenixd-password \
  -e API_KEY=my-secret-key \
  --network lnf \
  --restart unless-stopped \
  ghcr.io/dnjooiopa/phoenixd-proxy:latest
```

## Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `ENVIRONMENT` | No | - | Set to `production` to enable release mode and use `/app/data/proxy.db` as the database path |
| `PHOENIXD_URL` | Yes | - | Base URL of the phoenixd server (e.g. `http://phoenixd:9740`) |
| `PHOENIXD_PASSWORD` | Yes | - | HTTP password for phoenixd API authentication |
| `API_KEY` | Yes | - | API key for authenticating endpoint management requests |
| `ADDRESS` | No | - | Server bind address (e.g. `127.0.0.1`, `0.0.0.0`) |
| `PORT` | No | `8080` | Server listening port |

## API Reference

### Endpoints Management

All endpoint management routes require the `X-API-KEY` header.

#### List Endpoints

```bash
curl 'localhost:8080/endpoints' \
  -H 'X-API-KEY: my-secret-key'
```

Response: `200 OK`
```json
{
  "data": [
    {
      "id": 1,
      "url": "https://myapp.com/payment-hook",
      "created_at": "2026-03-15 10:00:00"
    }
  ]
}
```

#### Register Endpoint

```bash
curl -X POST 'localhost:8080/endpoints' \
  -H 'Content-Type: application/json' \
  -H 'X-API-KEY: my-secret-key' \
  -d '{"url": "https://myapp.com/payment-hook"}'
```

Response: `201 Created`
```json
{
  "data": {
    "id": 1,
    "url": "https://myapp.com/payment-hook",
    "created_at": "2026-03-15 10:00:00"
  }
}
```

Returns `409 Conflict` if the URL is already registered.

#### Delete Endpoint

```bash
curl -X DELETE 'localhost:8080/endpoints/1' \
  -H 'X-API-KEY: my-secret-key'
```

Response: `204 No Content`

Returns `404 Not Found` if the endpoint ID does not exist.

### Webhook Requests

Requires the `X-API-KEY` header.

#### List Webhook Requests

```bash
curl 'localhost:8080/webhook-requests?limit=10' \
  -H 'X-API-KEY: my-secret-key'
```

Response: `200 OK`
```json
{
  "data": [
    {
      "id": 1,
      "body": "{\"type\":\"payment_received\",\"amountSat\":1}",
      "content_type": "application/json",
      "signature": "<signature>",
      "created_at": "2026-03-15 10:00:00"
    }
  ]
}
```

| Query Param | Default | Description |
|-------------|---------|-------------|
| `limit`     | `100`   | Number of records to return (max `100`) |

### Phoenixd Proxy

Proxied routes require the `X-API-KEY` header. Requests are forwarded to the configured `PHOENIXD_URL` with HTTP Basic Auth.

#### Create Bolt11 Invoice

Creates a Bolt11 invoice. A Bolt11 invoice is a non-reusable, expirable payment request for Lightning.

```bash
curl -X POST 'localhost:8080/createinvoice' \
  -H 'X-API-KEY: my-secret-key' \
  -d description='my first invoice' \
  -d amountSat=100 \
  -d externalId=foobar \
  -d webhookUrl='https://my.webhook.net'
```

| Parameter | Required | Description |
|-----------|----------|-------------|
| `description` | Yes* | Description of the invoice (max 128 characters) |
| `descriptionHash` | Yes* | SHA256 hash of a description (alternative to `description`) |
| `amountSat` | No | Amount requested in satoshi. If not set, the invoice can be paid with any amount |
| `expirySeconds` | No | Invoice expiry in seconds (default `3600`) |
| `externalId` | No | Custom identifier to link the invoice to an external system |
| `webhookUrl` | No | Webhook URL to be notified when this payment is received |

\* Either `description` or `descriptionHash` is required.

Response: `200 OK`
```json
{
  "amountSat": 100,
  "paymentHash": "f419207c9edde9021ebfb6bd0df6bd0a6606ecaf935357cc2f362e30835c3765",
  "serialized": "lntb1u1pjlsjnq..."
}
```

### Webhook Receiver

The webhook endpoint does not require authentication. It receives the payload and forwards it to all registered endpoints asynchronously.

```bash
curl -X POST 'localhost:8080/webhook' \
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
  "data": {
    "status": "ok",
    "forwarded_to": 2
  }
}
```

The `Content-Type` and `X-Phoenix-Signature` headers are forwarded to all registered endpoints. Each incoming webhook request is also saved to the database and can be retrieved via the [Webhook Requests](#webhook-requests) API.

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
