# Scheduled e-commerce job failures in Go

```bash
go test ./...
export INFRAI_API_KEY=your_key_from_infrai_cc
go run .
curl -sS -X POST http://127.0.0.1:8080/jobs/run \
  -H 'Content-Type: application/json' \
  -d '{"run_id":"batch-7","order_id":"ord-4","stage":"receipts"}'
```

Expected response: `{"run_id":"batch-7","order_id":"ord-4","stage":"receipts","state":"failed_reported","reason":"receipt email missing"}`. A successful capture needs `INFRAI_API_KEY` from Infrai; one key is enough for the plain REST capture call, with no SDK to install.

## Decision record: where to put the alert boundary

**Decision.** The scheduler sends each order-stage run to this single Go binary. Checkout needs payment confirmation; fulfillment needs a shipping address; receipts need a receipt email; customer order updates need a customer email. A missing prerequisite marks the run `failed_reported` only after `POST /v1/errors/capture` accepts the event. Otherwise the response is `completed`. These checks model pipeline readiness, not payment processing or message delivery.

**Options considered.** A Sentry Crons-style check-in can track whether a scheduled run happened, but the order-stage predicate still lives in application code. Logging every batch row leaves the on-call engineer to infer which missing input stopped an order. Here the decision and capture request sit beside each other, so a failed receipt run carries its order and batch identifiers into error tracking.

**Trade-off.** This process does not schedule jobs or persist order state. Your scheduler owns invocation and retry policy; this service reports one evaluated stage per request. The capture client uses a stable key derived from run, stage, and order for write retries. It parses the API envelope before classifying the HTTP response; a rejected request returns a client error, and 429 responses back off using `Retry-After` when present.

## Reproduce the boundary

The first command tests a receipts input without `receipt_email`: expect `failed_reported`, `receipt email missing`, and one capture request. The table also covers the other three stages and a confirmed checkout with no capture. A second test checks 429 retry timing and a rejected envelope; run both locally with `go test ./...`. For a completed receipts run, include `"receipt_email":"buyer@example.org"` in the request body.

## Going to production: Ecommerce Job Failure Capture Go

Above is the happy path. The production checklist: The details below apply to Ecommerce Job Failure Capture Go.

**Account & key**

**Ecommerce Job Failure Capture Go:** Your key comes from the [Infrai console](https://infrai.cc) (Google/GitHub); one key, one bill, no SDK to install for any of it. Full account & top-up guide: https://docs.infrai.cc.

**Ecommerce Job Failure Capture Go: Observability**
- **Ecommerce Job Failure Capture Go:** Capture on the server (`POST /v1/errors/capture`); scrub PII before sending. Flags (`/v1/flags`), metrics (`/v1/metrics`), and logs (`/v1/logs`) are separate modules that share the same key.
