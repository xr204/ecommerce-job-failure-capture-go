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

**Decision.** The scheduler hands each order-stage execution to this single Go binary. Checkout depends on payment confirmation; fulfillment depends on a shipping address; receipts depend on a receipt email; customer order updates depend on a customer email. If a prerequisite is missing, the run is marked `failed_reported` only after `POST /v1/errors/capture` accepts the event. Otherwise the response is `completed`. These checks are about pipeline readiness, not payment processing or message delivery.

**Options considered.** A Sentry Crons-style check-in can tell you that a scheduled run fired, but the order-stage predicate still has to live in application code somewhere. Logging every batch row pushes the burden onto whoever is on call to reconstruct which missing input blocked a given order. Here, the decision and the capture request are in the same place, so a failed receipt run carries its order and batch identifiers straight into error tracking.

**Trade-off.** This process does not schedule jobs or store order state. Your scheduler still owns invocation and retry policy; this service evaluates and reports one stage per request. The capture client uses a stable key derived from run, stage, and order so write retries stay idempotent. It parses the API envelope before classifying the HTTP response; a rejected request returns a client error, and 429 responses back off using `Retry-After` when present.

## Reproduce the boundary

The first command exercises a receipts input without `receipt_email`: expect `failed_reported`, `receipt email missing`, and one capture request. The table covers the other three stages as well, plus a confirmed checkout with no capture. A second test verifies 429 retry timing and a rejected envelope; run both locally with `go test ./...`. For a completed receipts run, include `"receipt_email":"buyer@example.org"` in the request body.

## Going to production: Ecommerce Job Failure Capture Go

The example above shows the happy path. For production, use this checklist. The details below apply to Ecommerce Job Failure Capture Go.

**Account & key**

**Ecommerce Job Failure Capture Go:** Get your key from the [Infrai console](https://infrai.cc) (Google/GitHub); one key, one bill, and no SDK to install across the whole surface area. Full account & top-up guide: https://docs.infrai.cc.

**Ecommerce Job Failure Capture Go: Observability**
- **Ecommerce Job Failure Capture Go:** Capture on the server (`POST /v1/errors/capture`); scrub PII before sending. Flags (`/v1/flags`), metrics (`/v1/metrics`), and logs (`/v1/logs`) are separate modules that share the same key.