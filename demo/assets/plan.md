# Plan: Add Webhook Notifications

## Overview

When a deployment completes, send a webhook to notify external services.
This keeps dashboards, Slack bots, and monitoring tools in sync without polling.

## Design

### Webhook Payload

```json
{
  "event": "deploy.completed",
  "service": "api-gateway",
  "version": "v2.4.1",
  "environment": "production",
  "timestamp": "2026-03-07T14:30:00Z"
}
```

### Retry Strategy

Failed deliveries retry with exponential backoff:

- **1st retry:** 5 seconds
- **2nd retry:** 30 seconds
- **3rd retry:** 5 minutes
- **Max retries:** 3 attempts total

After 3 failures, mark the webhook as dead and alert the team.

### Endpoint Registration

Users register webhook URLs through the settings page.
Each endpoint stores:

- URL (validated on save)
- Secret key for HMAC signing
- Event filter (which events to receive)
- Active/inactive toggle

## Implementation Steps

1. [ ] Create `webhooks` table with endpoint config
2. [ ] Build registration API (`POST /webhooks`, `DELETE /webhooks/:id`)
3. [ ] Add delivery worker with retry queue
4. [ ] HMAC-sign payloads using endpoint secret
5. [ ] Add delivery log with status tracking
6. [ ] Write integration tests against httpbin
7. [ ] Update docs with webhook setup guide

## Open Questions

- Should we support batch delivery (multiple events per request)?
- What timeout should we use for webhook delivery? 10s seems reasonable.
- Do we need a UI for viewing delivery logs, or is the API enough?
