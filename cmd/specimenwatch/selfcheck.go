package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"specimen-transit-guard/internal/httpapi"
)

type selfcheckResult struct {
	Case struct {
		ID       string `json:"id"`
		Revision int64  `json:"revision"`
		State    string `json:"state"`
	} `json:"case"`
}

func runSelfcheck(address string) error {
	dir, err := os.MkdirTemp("", "specimenwatch-selfcheck-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	handler, err := buildHandler(dir)
	if err != nil {
		return err
	}
	server := httpapi.NewServer(address, handler)
	listener, err := httpapi.Listen(server)
	if err != nil {
		return fmt.Errorf("自检监听 %s: %w", address, err)
	}
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	baseURL := "http://" + listener.Addr().String()
	client := &http.Client{Timeout: 5 * time.Second}
	var registered selfcheckResult
	register := map[string]any{"shipment_code": "SELF-CHECK-001", "container_code": "BOX-001", "sample_category": "血液",
		"temperature_min_c": 2.0, "temperature_max_c": 8.0}
	if err := selfcheckRequest(client, http.MethodPost, baseURL+"/api/v1/transit-cases", register, map[string]string{
		"X-Actor": "selfcheck-receiver", "X-Request-ID": "selfcheck-register", "Idempotency-Key": "selfcheck-register"}, &registered); err != nil {
		return shutdownSelfcheck(server, done, err)
	}
	start := time.Date(2026, 8, 25, 10, 0, 0, 0, time.UTC)
	evidence := map[string]any{"transport_started_at": start, "transport_ended_at": start.Add(10 * time.Minute), "document_ref": "selfcheck://handoff/1",
		"digest_sha256": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", "readings": []map[string]any{
			{"recorded_at": start, "temperature_c": 5.0, "sensor_serial": "SENSOR-1", "source_batch": "BATCH-1"},
			{"recorded_at": start.Add(5 * time.Minute), "temperature_c": 5.5, "sensor_serial": "SENSOR-1", "source_batch": "BATCH-1"},
			{"recorded_at": start.Add(10 * time.Minute), "temperature_c": 6.0, "sensor_serial": "SENSOR-1", "source_batch": "BATCH-1"}}}
	var evidenced selfcheckResult
	headers := map[string]string{"X-Actor": "selfcheck-receiver", "X-Request-ID": "selfcheck-evidence", "Idempotency-Key": "selfcheck-evidence", "If-Match": fmt.Sprint(registered.Case.Revision)}
	if err := selfcheckRequest(client, http.MethodPost, baseURL+"/api/v1/transit-cases/"+registered.Case.ID+"/evidence", evidence, headers, &evidenced); err != nil {
		return shutdownSelfcheck(server, done, err)
	}
	var assessed selfcheckResult
	headers = map[string]string{"X-Actor": "selfcheck-quality", "X-Request-ID": "selfcheck-assess", "Idempotency-Key": "selfcheck-assess", "If-Match": fmt.Sprint(evidenced.Case.Revision)}
	if err := selfcheckRequest(client, http.MethodPost, baseURL+"/api/v1/transit-cases/"+registered.Case.ID+"/assessment", nil, headers, &assessed); err != nil {
		return shutdownSelfcheck(server, done, err)
	}
	var queried selfcheckResult
	if err := selfcheckRequest(client, http.MethodGet, baseURL+"/api/v1/transit-cases/"+registered.Case.ID, nil, nil, &queried); err != nil {
		return shutdownSelfcheck(server, done, err)
	}
	if queried.Case.State != "assessment_passed" || queried.Case.Revision != assessed.Case.Revision {
		return shutdownSelfcheck(server, done, fmt.Errorf("自检状态不符合预期: %+v", queried.Case))
	}
	return shutdownSelfcheck(server, done, nil)
}

func selfcheckRequest(client *http.Client, method, url string, body any, headers map[string]string, out any) error {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("%s %s 返回 %d: %s", method, url, resp.StatusCode, raw)
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

func shutdownSelfcheck(server *http.Server, done <-chan error, cause error) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil && cause == nil {
		cause = err
	}
	select {
	case <-done:
	case <-ctx.Done():
		if cause == nil {
			cause = ctx.Err()
		}
	}
	return cause
}
