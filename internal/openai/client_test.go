package openai

import "testing"

func TestParseConsensus(t *testing.T) {
	raw := "```json\n{\"reached\":true,\"score\":0.82,\"summary\":\"Team aligned\",\"rationale\":\"Most concerns resolved\",\"open_risks\":[\"monitor latency\"],\"required_next_action\":\"SRE to define rollback trigger by tomorrow\"}\n```"
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
	if len(consensus.OpenRisks) != 1 || consensus.OpenRisks[0] != "monitor latency" {
		t.Fatalf("unexpected open_risks: %#v", consensus.OpenRisks)
	}
	if consensus.RequiredNextAction == "" {
		t.Fatal("expected required_next_action to be parsed")
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
	_, err := parseConsensus(`{"reached":true,"score":0.9,"summary":"ok","open_risks":[],"required_next_action":"owner action"}`)
	if err == nil {
		t.Fatal("expected missing rationale error")
	}
}

func TestParseConsensusMissingOpenRisks(t *testing.T) {
	_, err := parseConsensus(`{"reached":true,"score":0.9,"summary":"ok","rationale":"x","required_next_action":"owner action"}`)
	if err == nil {
		t.Fatal("expected missing open_risks error")
	}
}

func TestParseConsensusMissingRequiredNextAction(t *testing.T) {
	_, err := parseConsensus(`{"reached":true,"score":0.9,"summary":"ok","rationale":"x","open_risks":[]}`)
	if err == nil {
		t.Fatal("expected missing required_next_action error")
	}
}

func TestParseConsensusPicksFirstBalancedJSONObject(t *testing.T) {
	raw := `prefix {"reached":true,"score":0.91,"summary":"ok","rationale":"uses {brace} in text","open_risks":["handoff"],"required_next_action":"PM decides option A today"} trailing {"ignored":true}`
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

func TestParseConsensusSkipsInvalidLeadingJSONObject(t *testing.T) {
	raw := `{"meta":true} {"reached":true,"score":0.67,"summary":"usable","rationale":"fallback object","open_risks":[],"required_next_action":"owner assigns next step"}`
	consensus, err := parseConsensus(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if consensus.Summary != "usable" {
		t.Fatalf("unexpected summary: %s", consensus.Summary)
	}
}

func TestParseConsensusOpenRisksAcceptsDelimitedString(t *testing.T) {
	raw := `{"reached":false,"score":0.6,"summary":"partial","rationale":"risk gap","open_risks":"risk-a, risk-b","required_next_action":"assign owner"}`
	consensus, err := parseConsensus(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(consensus.OpenRisks) != 2 {
		t.Fatalf("expected 2 open risks, got %#v", consensus.OpenRisks)
	}
}

func TestParseConsensusRejectsInvalidOpenRisksType(t *testing.T) {
	raw := `{"reached":false,"score":0.6,"summary":"partial","rationale":"risk gap","open_risks":{"a":"b"},"required_next_action":"assign owner"}`
	_, err := parseConsensus(raw)
	if err == nil {
		t.Fatal("expected invalid open_risks type error")
	}
}

func TestParseConsensusFromCodeFenceWithoutLanguage(t *testing.T) {
	raw := "```\n{\"reached\":true,\"score\":0.8,\"summary\":\"ok\",\"rationale\":\"done\",\"open_risks\":[],\"required_next_action\":\"owner does next\"}\n```"
	consensus, err := parseConsensus(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
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

func TestParseOpeningSpeakerIDSkipsInvalidLeadingJSONObject(t *testing.T) {
	id, err := parseOpeningSpeakerID(`{"meta":"x"} {"persona_id":"operator"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "operator" {
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

func TestParseOpeningSpeakerIDFromCodeFenceWithoutLanguage(t *testing.T) {
	id, err := parseOpeningSpeakerID("```\nsecurity_analyst\n```")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "security_analyst" {
		t.Fatalf("unexpected persona id: %s", id)
	}
}

func TestParseOpeningSpeakerIDRejectsMissingID(t *testing.T) {
	_, err := parseOpeningSpeakerID(`{"reason":"missing id"}`)
	if err == nil {
		t.Fatal("expected missing persona id error")
	}
}
