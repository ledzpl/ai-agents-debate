package openai

import "testing"

func TestParseConsensus(t *testing.T) {
	raw := "```json\n{\"reached\":true,\"score\":0.82,\"summary\":\"Team aligned\",\"rationale\":\"Most concerns resolved\"}\n```"
	consensus, err := parseConsensus(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !consensus.Reached {
		t.Fatal("expected reached=true")
	}
	if consensus.Score != 0.82 {
		t.Fatalf("unexpected score: %v", consensus.Score)
	}
	if consensus.Summary != "Team aligned" {
		t.Fatalf("unexpected summary: %s", consensus.Summary)
	}
}

func TestParseConsensusInvalid(t *testing.T) {
	_, err := parseConsensus("not-json")
	if err == nil {
		t.Fatal("expected parse error")
	}
}

func TestParseConsensusMissingRequiredKey(t *testing.T) {
	_, err := parseConsensus(`{"reached":true,"summary":"ok"}`)
	if err == nil {
		t.Fatal("expected missing key error")
	}
}

func TestParseConsensusMissingRationale(t *testing.T) {
	_, err := parseConsensus(`{"reached":true,"score":0.9,"summary":"ok"}`)
	if err == nil {
		t.Fatal("expected missing rationale error")
	}
}

func TestParseConsensusPicksFirstBalancedJSONObject(t *testing.T) {
	raw := `prefix {"reached":true,"score":0.91,"summary":"ok","rationale":"uses {brace} in text"} trailing {"ignored":true}`
	consensus, err := parseConsensus(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !consensus.Reached {
		t.Fatal("expected reached=true")
	}
	if consensus.Score != 0.91 {
		t.Fatalf("unexpected score: %v", consensus.Score)
	}
	if consensus.Summary != "ok" {
		t.Fatalf("unexpected summary: %s", consensus.Summary)
	}
}

func TestParseOpeningSpeakerIDFromJSONObject(t *testing.T) {
	id, err := parseOpeningSpeakerID(`{"persona_id":"security_analyst","reason":"incident response context"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "security_analyst" {
		t.Fatalf("unexpected persona id: %s", id)
	}
}

func TestParseOpeningSpeakerIDFromSingleLineFallback(t *testing.T) {
	id, err := parseOpeningSpeakerID("architect")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "architect" {
		t.Fatalf("unexpected persona id: %s", id)
	}
}

func TestParseOpeningSpeakerIDRejectsMissingID(t *testing.T) {
	_, err := parseOpeningSpeakerID(`{"reason":"missing id"}`)
	if err == nil {
		t.Fatal("expected missing persona id error")
	}
}
