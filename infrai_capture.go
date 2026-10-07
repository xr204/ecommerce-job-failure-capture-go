package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

type captureClient struct {
	baseURL string
	key     string
	http    *http.Client
	wait    func(context.Context, time.Duration) error
}

type captureFailure struct {
	Code string `json:"code"`
	Hint string `json:"hint"`
}

type captureEnvelope struct {
	OK       bool            `json:"ok"`
	Data     json.RawMessage `json:"data"`
	Error    *captureFailure `json:"error"`
	Metadata json.RawMessage `json:"metadata"`
}

type apiError struct {
	Status int
	Code   string
	Hint   string
}

func (e *apiError) Error() string { return fmt.Sprintf("capture rejected: %s: %s", e.Code, e.Hint) }

func sleepContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c captureClient) capture(ctx context.Context, job jobRun, reason string) error {
	payload := map[string]any{
		"title":       job.Stage + " job failed",
		"message":     reason,
		"level":       "error",
		"exception":   reason,
		"fingerprint": []string{"ecommerce-job", job.Stage},
		"context":     map[string]string{"order_id": job.OrderID, "run_id": job.RunID, "stage": job.Stage},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	for attempt := 0; attempt < 4; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/errors/capture", bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+c.key)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", "ecommerce-job:"+job.RunID+":"+job.Stage+":"+job.OrderID)
		res, err := c.http.Do(req)
		if err != nil {
			return err
		}
		var envelope captureEnvelope
		decodeErr := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&envelope)
		res.Body.Close()
		if decodeErr != nil {
			return fmt.Errorf("decode capture response: %w", decodeErr)
		}
		if res.StatusCode == http.StatusTooManyRequests && attempt < 3 {
			delay := time.Duration(1<<attempt) * time.Second
			if seconds, err := strconv.Atoi(res.Header.Get("Retry-After")); err == nil && seconds >= 0 {
				delay = time.Duration(seconds) * time.Second
			}
			if err := c.wait(ctx, delay); err != nil {
				return err
			}
			continue
		}
		if !envelope.OK {
			if envelope.Error != nil {
				return &apiError{Status: res.StatusCode, Code: envelope.Error.Code, Hint: envelope.Error.Hint}
			}
			return &apiError{Status: res.StatusCode, Code: "capture_rejected"}
		}
		if res.StatusCode < 200 || res.StatusCode >= 300 {
			return fmt.Errorf("capture HTTP status %d", res.StatusCode)
		}
		return nil
	}
	return errors.New("capture retry limit reached")
}
