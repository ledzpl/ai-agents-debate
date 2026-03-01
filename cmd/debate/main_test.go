package main

import (
	"testing"

	"debate/internal/config"
)

func TestParseRuntimeOptionsDefaults(t *testing.T) {
	opts, err := parseRuntimeOptions(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.personaPath != config.DefaultPersonaPath {
		t.Fatalf("unexpected default persona path: %s", opts.personaPath)
	}
	if opts.webMode {
		t.Fatal("expected webMode to be false by default")
	}
	if opts.addr != "" {
		t.Fatalf("expected empty addr by default, got %q", opts.addr)
	}
}

func TestParseRuntimeOptionsPersonasFlag(t *testing.T) {
	opts, err := parseRuntimeOptions([]string{"--personas", "./exmaples/personas.pm.json"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.personaPath != "./exmaples/personas.pm.json" {
		t.Fatalf("unexpected persona path: %s", opts.personaPath)
	}
}

func TestParseRuntimeOptionsPersonaAlias(t *testing.T) {
	opts, err := parseRuntimeOptions([]string{"--persona", "./custom.json"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.personaPath != "./custom.json" {
		t.Fatalf("unexpected persona path: %s", opts.personaPath)
	}
}

func TestParseRuntimeOptionsRejectsPositionalArgs(t *testing.T) {
	_, err := parseRuntimeOptions([]string{"unexpected"})
	if err == nil {
		t.Fatal("expected error for positional args")
	}
}

func TestParseRuntimeOptionsWebModeAndAddr(t *testing.T) {
	opts, err := parseRuntimeOptions([]string{"--web", "--addr", "  :8090  "})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !opts.webMode {
		t.Fatal("expected webMode=true")
	}
	if opts.addr != ":8090" {
		t.Fatalf("unexpected addr: %q", opts.addr)
	}
}
