# Advanced Request Features

This document covers the Phase 3 request and token features currently implemented in HookWatch:

- webhook replay
- request diffing
- OpenAPI generation from captured traffic
- token persistence
- webhook signature validation

## Webhook Replay

Replay sends a previously captured request to a new target URL from the server side.

Endpoint:

```text
POST /api/tokens/{tokenId}/requests/{requestId}/replay
```

Request body:

```json
{
  "url": "https://target.example.com/webhook",
  "preserve_headers": true,
  "additional_headers": {
    "X-HookWatch-Replay": "true"
  }
}
```

Behavior:

- reuses the captured HTTP method and request body
- appends the original captured query string to the target URL
- preserves captured headers when `preserve_headers=true`
- strips hop-by-hop transport headers such as `Host`, `Connection`, and `Content-Length`
- returns the upstream status code, response headers, a truncated response body, and request duration

The token detail page exposes replay from the `Advanced` tab for the selected request.

## Request Diffing

Diffing compares two captured requests under the same token.

Endpoint:

```text
GET /api/tokens/{tokenId}/requests/diff?left={requestId}&right={requestId}
```

The diff response compares:

- method
- URL
- query parameters
- headers
- form data
- request body
- IP address
- user agent
- received timestamp

JSON bodies and structured request fields are normalized before comparison when possible. The frontend renders the diff from the token detail page's `Advanced` tab.

## OpenAPI Generation

HookWatch can generate a best-effort OpenAPI 3.0 document from captured requests for a token.

Endpoint:

```text
GET /api/tokens/{tokenId}/openapi.json
```

Behavior:

- groups requests by normalized path and method
- infers query parameters from captured query strings
- infers request-body schemas from JSON payloads and parsed form data
- emits a downloadable OpenAPI JSON document

This output is documentation derived from observed traffic. It is not a source-of-truth contract and may omit edge cases that have not been captured yet.

## Persistent Tokens

Authenticated owners and admins can mark a token as persistent so it does not expire automatically.

Token field:

```json
{
  "persistent": true
}
```

Behavior:

- persistent tokens bypass normal TTL refresh and cleanup expiration
- persistent tokens require authentication
- only the token owner or an admin can change persistence
- in `auth-mode=none`, persistent tokens are rejected

The token settings modal exposes the persistence toggle when the current user is allowed to change it.

## Webhook Signature Validation

HookWatch can validate provider signatures when requests are captured. Validation happens once at ingest time and is stored on the captured request.

Supported providers:

- GitHub
- Stripe

Token configuration fields:

```json
{
  "signature_provider": "github",
  "signature_secret": "topsecret"
}
```

Behavior:

- validation runs during webhook capture
- the request stores a validation status of `valid`, `invalid`, or `unknown`
- request detail shows the provider, status, and validation error when present
- leaving `signature_provider` empty disables validation

Notes:

- GitHub uses `X-Hub-Signature-256`
- Stripe uses `Stripe-Signature`
- the token response only indicates whether a signature secret is configured; it does not return the stored secret

## Verification

These features are covered by Go API and store tests:

- replay request endpoint
- diff endpoint
- OpenAPI generation endpoint
- persistent token expiry behavior
- signature validation capture behavior
