# Design Document: Hybrid Local Buffer and Fallback Queue for Click Analytics

## Problem Statement
The click analytics tracking currently pipelines `RPop` commands to Redis every 5 seconds (Task 8), which rapidly exhausts the 500,000 requests/day quota of Upstash's free tier. Additionally, if Redis is down or rate-limited, the application fails to save click analytics events, resulting in data loss.

## Proposed Architecture
We will implement a **Hybrid Local Buffer and Fallback Queue** in the Go backend.

### 1. In-Memory Event Channel
We will expose a global (or handler-scoped) thread-safe buffered channel of `AnalyticsEvent` in the backend:
```go
var AnalyticsBufferChan = make(chan AnalyticsEvent, 5000)
```
In the redirect handler `RedirectFallback`, instead of pushing the event directly to Redis, we perform a non-blocking select to queue it locally:
```go
select {
case AnalyticsBufferChan <- event:
    // Queued successfully
default:
    slog.Warn("analytics local buffer queue is full, dropping event")
}
```

### 2. Local Buffer Worker (`LocalBufferWorker`)
We will create a lightweight background service that manages a buffer slice of events:
- Read incoming events from `AnalyticsBufferChan`.
- Maintain a local buffer slice of events.
- **Flush Condition A**: If the buffer slice reaches **100 events**, try to push them to Redis in **a single `LPUSH` batch command**. If Redis fails, fall back and save them directly to PostgreSQL.
- **Flush Condition B**: If a **10-second ticker** fires and the buffer slice is under 100 events, **bypass Redis completely** and write them directly to PostgreSQL.
- **Zero Events**: If the buffer is empty when the ticker fires, do nothing.

## Resiliency and Performance
- **Reduced Redis Quota Usage**: Under high traffic, events are batched into 100-event single-commands. Under low traffic, Redis is completely bypassed.
- **DB Write Protection**: The database is only written to in batches (up to 100 rows per transaction or once every 10 seconds), preventing direct single-insert database spikes.
- **Low Latency**: The HTTP redirect response does not wait for database or Redis writes, making it execute in nanoseconds.
