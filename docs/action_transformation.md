# Action Pipeline & Transformations

HookWatch processes actions sequentially after capturing a webhook request. Each action operates on a shared **pipeline context** that carries mutable request data through the chain. Transform actions modify this context; downstream actions (e.g. forward) read from it.

## Pipeline Context

When a request is captured, the pipeline context is initialized from it:

| Field         | Source                          |
|---------------|---------------------------------|
| `Method`      | HTTP method of captured request |
| `Body`        | Request body content            |
| `ContentType` | `Content-Type` header value     |
| `Headers`     | All request headers (map)       |
| `Query`       | Query string (JSON-encoded)     |
| `IP`          | Client IP address               |
| `URL`         | Request URL path                |

Actions execute in sort order. If any action fails or a filter stops the pipeline, remaining actions are skipped.

## Action Types

### Forward

Sends the (potentially transformed) request to an external URL.

| Config Field | Type   | Required | Default          | Constraints              |
|-------------|--------|----------|------------------|--------------------------|
| `url`       | string | yes      | —                | Must be `http` or `https` |
| `method`    | string | no       | Same as original | GET, POST, PUT, PATCH, DELETE |
| `headers`   | object | no       | —                | Key-value pairs override pipeline headers |
| `timeout`   | int    | no       | 10               | 1–30 seconds             |

The forward executor reads `Body`, `Headers`, and `ContentType` from the pipeline context, so any upstream transform is reflected in the forwarded request.

### Filter

Evaluates a condition against the request. If the condition is **not met**, the pipeline stops and all remaining actions are marked `skipped`.

| Config Field | Type   | Required | Default | Notes |
|-------------|--------|----------|---------|-------|
| `field`     | string | yes      | —       | See field reference below |
| `operator`  | string | yes      | —       | `equals`, `contains`, `matches`, `exists` |
| `value`     | string | conditional | —    | Required for all operators except `exists` |
| `negate`    | bool   | no       | false   | Inverts the match — stops pipeline when condition **matches** |

**Field reference:**

| Field               | Resolves to                        |
|---------------------|------------------------------------|
| `method`            | HTTP method (e.g. `POST`)          |
| `ip`                | Client IP address                  |
| `content`           | Request body                       |
| `header.<Name>`     | Header value (case-insensitive)    |
| `query.<Name>`      | Query parameter value              |

**Operator reference:**

| Operator   | Behavior                                  |
|------------|-------------------------------------------|
| `equals`   | Exact string match                        |
| `contains` | Substring match                           |
| `matches`  | Go `regexp` match against field value     |
| `exists`   | True if the field is present and non-empty |

### Delay

Pauses the pipeline for a specified duration before continuing to the next action.

| Config Field  | Type | Required | Default | Constraints        |
|--------------|------|----------|---------|--------------------|
| `duration_ms` | int  | yes      | —       | 100–30,000 ms      |

Respects the overall pipeline timeout (2 minutes). If the pipeline context is cancelled during the delay, execution stops.

### Transform

Mutates the pipeline context for downstream actions. Transform does **not** alter the HTTP response already sent to the original caller — it only affects what subsequent actions (like forward) see.

| Config Field   | Type   | Required | Default | Constraints          |
|---------------|--------|----------|---------|----------------------|
| `status`      | *int   | no       | —       | 100–999 (recorded in log, does not change capture response) |
| `content_type` | *string | no      | —       | Overrides pipeline `ContentType` |
| `body`        | *string | no      | —       | Overrides pipeline `Body` |

At least one field must be provided. All fields are optional and use pointer semantics — only non-null fields are applied.

**What each field does:**

- **`body`** — Replaces `pctx.Body`. Any downstream forward action will send this body instead of the original request content.
- **`content_type`** — Replaces `pctx.ContentType`. Downstream forward actions will use this as the `Content-Type` header (unless explicitly overridden in the forward config).
- **`status`** — Recorded in the action log for observability. Does not modify the pipeline context or the captured response.

## Example Pipeline

A typical pipeline that filters, transforms, and forwards:

```
1. Filter    — Only continue if method is POST
2. Transform — Replace body with a JSON envelope, set content_type to application/json
3. Delay     — Wait 500ms (rate limiting)
4. Forward   — Send transformed request to https://api.example.com/ingest
```

## Execution Details

- Actions run in a background goroutine with a **2-minute** overall timeout.
- The worker pool has configurable concurrency (default: 50 concurrent pipelines).
- Each action produces an `ActionLog` entry with status (`pending` → `running` → `success`/`failed`/`skipped`) and timing data.
- Completion events are published via SSE (`action.completed`) for real-time UI updates.
