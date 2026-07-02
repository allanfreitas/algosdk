package job

import (
	"encoding/json"
	"testing"
)

func TestDecodePayload(t *testing.T) {
	type TestPayload struct {
		NfseID string `json:"nfse_id"`
		Count  int    `json:"count"`
	}

	payload := TestPayload{
		NfseID: "123e4567-e89b-12d3-a456-426614174000",
		Count:  42,
	}

	bytes, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("failed to marshal test payload: %v", err)
	}

	jobObj := Job{
		Payload: bytes,
	}

	decoded, err := DecodePayload[TestPayload](jobObj)
	if err != nil {
		t.Fatalf("DecodePayload failed: %v", err)
	}

	if decoded.NfseID != payload.NfseID {
		t.Errorf("decoded NfseID = %q, want %q", decoded.NfseID, payload.NfseID)
	}

	if decoded.Count != payload.Count {
		t.Errorf("decoded Count = %d, want %d", decoded.Count, payload.Count)
	}
}

func TestNewClientDefaults(t *testing.T) {
	client := New(Config{})

	if client.cfg.JobsTable != "jobs" {
		t.Errorf("expected JobsTable to default to 'jobs', got %q", client.cfg.JobsTable)
	}

	if client.cfg.AttemptsTable != "job_attempts" {
		t.Errorf("expected AttemptsTable to default to 'job_attempts', got %q", client.cfg.AttemptsTable)
	}
}
