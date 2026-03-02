package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

func decodeDebateRequest(body io.Reader) (debateRequest, error) {
	var req debateRequest
	if err := decodeStrictJSON(body, &req); err != nil {
		return debateRequest{}, fmt.Errorf("invalid request body: %w", err)
	}

	req.Problem = strings.TrimSpace(req.Problem)
	if req.Problem == "" {
		return debateRequest{}, errors.New("problem is required")
	}
	return req, nil
}

func decodeStreamStopRequest(body io.Reader) (streamStopRequest, error) {
	var req streamStopRequest
	if err := decodeStrictJSON(body, &req); err != nil {
		return streamStopRequest{}, fmt.Errorf("invalid request body: %w", err)
	}
	req.RunID = strings.TrimSpace(req.RunID)
	if req.RunID == "" {
		return streamStopRequest{}, errors.New("run_id is required")
	}
	return req, nil
}

func writeSSE(w io.Writer, flusher http.Flusher, event string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "event: %s\n", event); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
		return err
	}
	flusher.Flush()
	return nil
}

func methodNotAllowed(w http.ResponseWriter, allowed string) {
	w.Header().Set("Allow", allowed)
	writeError(w, http.StatusMethodNotAllowed, "method not allowed")
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
