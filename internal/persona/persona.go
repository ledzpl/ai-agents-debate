package persona

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

const (
	MinPersonas = 2
	MaxPersonas = 12
)

type Persona struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	MasterName    string   `json:"master_name,omitempty"`
	Role          string   `json:"role"`
	Stance        string   `json:"stance,omitempty"`
	Style         string   `json:"style,omitempty"`
	Expertise     []string `json:"expertise,omitempty"`
	SignatureLens []string `json:"signature_lens,omitempty"`
	Constraints   []string `json:"constraints,omitempty"`
}

func LoadFromFile(path string) ([]Persona, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read persona file: %w", err)
	}

	var personas []Persona
	if err := json.Unmarshal(data, &personas); err != nil {
		return nil, fmt.Errorf("parse persona json: %w", err)
	}

	normalized, err := NormalizeAndValidate(personas)
	if err != nil {
		return nil, err
	}
	return normalized, nil
}

func NormalizeAndValidate(personas []Persona) ([]Persona, error) {
	if len(personas) < MinPersonas {
		return nil, fmt.Errorf("at least %d personas are required", MinPersonas)
	}
	if len(personas) > MaxPersonas {
		return nil, fmt.Errorf("at most %d personas are allowed", MaxPersonas)
	}

	seen := make(map[string]struct{}, len(personas))
	out := make([]Persona, 0, len(personas))

	for i, p := range personas {
		p.ID = strings.TrimSpace(p.ID)
		p.Name = strings.TrimSpace(p.Name)
		p.MasterName = strings.TrimSpace(p.MasterName)
		p.Role = strings.TrimSpace(p.Role)
		p.Stance = strings.TrimSpace(p.Stance)
		p.Style = strings.TrimSpace(p.Style)

		if p.ID == "" {
			return nil, fmt.Errorf("persona[%d].id is required", i)
		}
		if p.Name == "" {
			return nil, fmt.Errorf("persona[%d].name is required", i)
		}
		if p.Role == "" {
			return nil, fmt.Errorf("persona[%d].role is required", i)
		}
		if _, exists := seen[p.ID]; exists {
			return nil, fmt.Errorf("duplicate persona id: %s", p.ID)
		}
		seen[p.ID] = struct{}{}

		p.Expertise = trimNonEmpty(p.Expertise)
		p.SignatureLens = trimNonEmpty(p.SignatureLens)
		p.Constraints = trimNonEmpty(p.Constraints)
		if p.Stance == "" {
			p.Stance = "neutral"
		}

		out = append(out, p)
	}

	return out, nil
}

func DisplayName(p Persona) string {
	name := strings.TrimSpace(p.Name)
	master := strings.TrimSpace(p.MasterName)
	switch {
	case name == "":
		return master
	case master == "":
		return name
	default:
		return fmt.Sprintf("%s (%s)", name, master)
	}
}

func trimNonEmpty(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
