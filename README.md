# OpenTap

Self-hosted event analytics. Send events over HTTP, get user profiles, auto-discovered
event schemas, audience segments and a dashboard. You own all the data.

```
HTTP client → API (Go) → NATS JetStream → worker → TimescaleDB
                                             └→ Redis (rate limits, caches, distinct counts)
Dashboard  → Caddy → API
```

## Running in production

```sh
export ADMIN_TOKEN=$(openssl rand -hex 32) DOMAIN=analytics.example.com
docker compose up -d --build
```

Caddy serves the dashboard and the API on the same origin with automatic HTTPS.
Migrations run automatically when the API or worker starts.

## Development

```sh
cp .env.example .env   # set ADMIN_TOKEN
make dev               # postgres, nats, redis
make api               # in one terminal
make worker            # in another
make dashboard         # http://localhost:5173, proxies /v1 to the API
make test
```

## First steps

```sh
# Create a project and an API key (admin token)
curl -X POST localhost:8080/v1/projects -H "Authorization: Bearer $ADMIN_TOKEN" -d '{"name":"My App"}'
curl -X POST localhost:8080/v1/projects/<project_id>/keys -H "Authorization: Bearer $ADMIN_TOKEN" -d '{"label":"prod"}'

# Track events (project API key)
curl -X POST localhost:8080/v1/events -H "Authorization: Bearer ot_..." \
  -d '{"user_id":"user_123","event_name":"purchase","properties":{"amount":120},"user_properties":{"plan":"pro"}}'
```

`user_properties` is optional and merged into the user's profile, which `user_property` segment rules match against.
Events may carry their own `id` (UUID) and `timestamp`; resending the same event is then a no-op.
In the dashboard, enter both credentials under **Settings**.

### Identity

Track anonymous visitors under any ID, then link it to the real user once known:

```sh
curl -X POST localhost:8080/v1/identify -H "Authorization: Bearer ot_..." \
  -d '{"anonymous_id":"3f1c...","user_id":"user_123"}'
```

The anonymous user's events and properties are merged into `user_123`, and events that still
arrive under the anonymous ID are attributed to `user_123` too.

### API keys

Keys are either `full` (read and write) or `ingest` (send events and identify only). Use ingest
keys in browsers and mobile apps, where the key is visible to users.

### Web SDK

```html
<script src="https://analytics.example.com/opentap.js"></script>
<script>
  OpenTap.init({ apiKey: 'ot_<ingest key>', host: 'https://analytics.example.com' });
  OpenTap.track('page_viewed', { page: location.pathname });
  OpenTap.identify('user_123', { plan: 'pro' }); // after login
  OpenTap.reset();                               // after logout
</script>
```

Events are batched every 2 seconds and flushed when the page is hidden.

## API

| Auth | Endpoint |
|---|---|
| none | `GET /health`, `GET /opentap.js` |
| admin token | `POST/GET /v1/projects`, `GET /v1/projects/:id` |
| admin token | `POST/GET /v1/projects/:id/keys`, `DELETE /v1/projects/:id/keys/:key_id` |
| any key | `POST /v1/events`, `POST /v1/events/batch` (max 100), `POST /v1/identify` |
| full key | `GET /v1/events`, `GET /v1/events/:id` |
| full key | `GET /v1/users`, `GET /v1/users/:id`, `GET /v1/users/:id/events` |
| full key | `POST/GET /v1/segments`, `GET/PUT/DELETE /v1/segments/:id`, `GET /v1/segments/:id/users`, `GET /v1/segments/:id/export` (CSV) |
| full key | `GET /v1/schemas`, `GET /v1/schemas/:event_name` |
| full key | `GET /v1/analytics/events/count`, `GET /v1/analytics/events/top`, `GET /v1/analytics/users/active` |
| full key | `GET /v1/analytics/funnel?steps=a,b,c&time_window=30d` |
| full key | `GET /v1/analytics/retention?start_event=signup&return_event=login&interval=week&periods=8` |

Lists are cursor-paginated (`limit`, `cursor`). Errors are `{"error":{"code","message"}}`.

### Segment conditions

```json
{
  "operator": "AND",
  "rules": [
    {
      "type": "event", "event_name": "purchase", "time_window": "30d",
      "property_filters": [{ "property": "amount", "operator": "gt", "value": 100 }],
      "count_operator": "gte", "count_value": 1
    },
    { "type": "user_property", "property": "plan", "operator": "eq", "value": "pro" }
  ]
}
```

Operators: `eq neq gt gte lt lte contains not_contains is_set is_not_set`.
Time windows: `1h 24h 7d 30d 90d all`. Numeric comparisons ignore non-numeric values.
Segments are recomputed by the worker every `SEGMENT_INTERVAL`.

### Segment webhooks

Give a segment a `webhook_url` and, after each recompute that changes membership, the worker POSTs
(in batches of 1000 users, retried 3 times):

```json
{ "segment_id": "...", "segment_name": "Pro", "user_count": 1523, "computed_at": "...",
  "added": ["user_123"], "removed": ["user_456"] }
```

With `WEBHOOK_SECRET` set, each request carries `X-OpenTap-Signature: sha256=<hex HMAC-SHA256 of the body>`.
Use `GET /v1/segments/:id/export` for a full CSV resync.

## Delivery guarantees

The API returns `202` once NATS has the event. `WORKER_CONCURRENCY` handlers ingest in parallel,
so one user's events may be applied out of order (first and last seen stay exact). The worker
retries database failures with backoff for as long as it takes, and ingestion is idempotent, so
redeliveries never duplicate events. Events the database rejects as invalid go to the `events.dlq` subject
after `DLQ_MAX_RETRIES` attempts. Inspect them with `nats stream view EVENTS --subject events.dlq`.

Configuration is documented in [.env.example](.env.example).
