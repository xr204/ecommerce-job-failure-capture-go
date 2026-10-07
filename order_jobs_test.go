package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestJobDecisionAndCapture(t *testing.T) {
	tests := []struct {
		name, input, state, reason string
		calls                      int
	}{
		{"checkout confirmed", `{"run_id":"batch-7","order_id":"ord-4","stage":"checkout","payment_confirmed":true}`, "completed", "", 0},
		{"checkout pending", `{"run_id":"batch-7","order_id":"ord-4","stage":"checkout"}`, "failed_reported", "payment confirmation missing", 1},
		{"fulfillment", `{"run_id":"batch-7","order_id":"ord-4","stage":"fulfillment"}`, "failed_reported", "shipping address missing", 1},
		{"receipts", `{"run_id":"batch-7","order_id":"ord-4","stage":"receipts"}`, "failed_reported", "receipt email missing", 1},
		{"customer updates", `{"run_id":"batch-7","order_id":"ord-4","stage":"customer_order_updates"}`, "failed_reported", "customer email missing", 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method != http.MethodPost || r.URL.Path != "/v1/errors/capture" || r.Header.Get("Authorization") != "Bearer test-key" || r.Header.Get("Idempotency-Key") == "" {
					t.Errorf("unexpected capture request: %s %s", r.Method, r.URL.Path)
				}
				var payload map[string]any
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || payload["message"] != tc.reason {
					t.Errorf("unexpected payload: %v, %v", payload, err)
				}
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`{"ok":true,"data":{},"error":null,"metadata":{}}`))
			}))
			defer backend.Close()
			client := captureClient{baseURL: backend.URL, key: "test-key", http: backend.Client(), wait: sleepContext}
			recorder := httptest.NewRecorder()
			jobHandler(client)(recorder, httptest.NewRequest(http.MethodPost, "/jobs/run", strings.NewReader(tc.input)))
			var result jobResult
			if recorder.Code != http.StatusOK || json.Unmarshal(recorder.Body.Bytes(), &result) != nil || result.State != tc.state || result.Reason != tc.reason || calls != tc.calls {
				t.Fatalf("status=%d result=%+v calls=%d", recorder.Code, result, calls)
			}
		})
	}
}

func TestCaptureRetryAndEnvelopeRejection(t *testing.T) {
	requests := 0
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if requests == 1 {
			w.Header().Set("Retry-After", "2")
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"ok":false,"error":{"code":"rate_limited"},"data":null,"metadata":{}}`))
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"ok":false,"error":{"code":"invalid_input","hint":"check input"},"data":null,"metadata":{}}`))
	}))
	defer backend.Close()
	var waited time.Duration
	client := captureClient{baseURL: backend.URL, key: "test-key", http: backend.Client(), wait: func(_ context.Context, d time.Duration) error { waited = d; return nil }}
	recorder := httptest.NewRecorder()
	jobHandler(client)(recorder, httptest.NewRequest(http.MethodPost, "/jobs/run", strings.NewReader(`{"run_id":"batch-7","order_id":"ord-4","stage":"receipts"}`)))
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "invalid_input") || requests != 2 || waited != 2*time.Second {
		t.Fatalf("status=%d body=%s attempts=%d waited=%s", recorder.Code, recorder.Body.String(), requests, waited)
	}
}
