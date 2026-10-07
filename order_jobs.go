package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

type jobRun struct {
	RunID            string `json:"run_id"`
	OrderID          string `json:"order_id"`
	Stage            string `json:"stage"`
	PaymentConfirmed bool   `json:"payment_confirmed"`
	ShippingAddress  string `json:"shipping_address"`
	ReceiptEmail     string `json:"receipt_email"`
	CustomerEmail    string `json:"customer_email"`
}

type jobResult struct {
	RunID   string `json:"run_id"`
	OrderID string `json:"order_id"`
	Stage   string `json:"stage"`
	State   string `json:"state"`
	Reason  string `json:"reason,omitempty"`
}

func decision(job jobRun) string {
	switch job.Stage {
	case "checkout":
		if !job.PaymentConfirmed {
			return "payment confirmation missing"
		}
	case "fulfillment":
		if strings.TrimSpace(job.ShippingAddress) == "" {
			return "shipping address missing"
		}
	case "receipts":
		if strings.TrimSpace(job.ReceiptEmail) == "" {
			return "receipt email missing"
		}
	case "customer_order_updates":
		if strings.TrimSpace(job.CustomerEmail) == "" {
			return "customer email missing"
		}
	default:
		return ""
	}
	return ""
}

func validStage(stage string) bool {
	switch stage {
	case "checkout", "fulfillment", "receipts", "customer_order_updates":
		return true
	}
	return false
}

func jobHandler(client captureClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", "POST")
			http.Error(w, "POST required", http.StatusMethodNotAllowed)
			return
		}
		var job jobRun
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&job); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if job.RunID == "" || job.OrderID == "" || !validStage(job.Stage) {
			http.Error(w, "run_id, order_id and a valid stage are required", http.StatusBadRequest)
			return
		}
		result := jobResult{RunID: job.RunID, OrderID: job.OrderID, Stage: job.Stage, State: "completed"}
		if reason := decision(job); reason != "" {
			if err := client.capture(r.Context(), job, reason); err != nil {
				var rejected *apiError
				if errors.As(err, &rejected) && rejected.Status >= 400 && rejected.Status < 500 {
					http.Error(w, err.Error(), rejected.Status)
				} else {
					http.Error(w, "capture failed: "+err.Error(), http.StatusBadGateway)
				}
				return
			}
			result.State, result.Reason = "failed_reported", reason
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	}
}
