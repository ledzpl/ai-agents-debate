package persona

import "testing"

func TestNormalizeAndValidate(t *testing.T) {
	personas := []Persona{
		{ID: " architect ", Name: " Architect ", Role: " Design ", Stance: " ", Expertise: []string{" systems ", ""}, SignatureLens: []string{" growth loops ", ""}},
		{ID: "operator", Name: "Operator", Role: "Reliability"},
	}

	normalized, err := NormalizeAndValidate(personas)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got := normalized[0].ID; got != "architect" {
		t.Fatalf("unexpected id: %s", got)
	}
	if got := normalized[0].Stance; got != "neutral" {
		t.Fatalf("unexpected stance: %s", got)
	}
	if len(normalized[0].Expertise) != 1 || normalized[0].Expertise[0] != "systems" {
		t.Fatalf("unexpected expertise: %#v", normalized[0].Expertise)
	}
	if len(normalized[0].SignatureLens) != 1 || normalized[0].SignatureLens[0] != "growth loops" {
		t.Fatalf("unexpected signature lens: %#v", normalized[0].SignatureLens)
	}
}

func TestNormalizeAndValidateDuplicateID(t *testing.T) {
	_, err := NormalizeAndValidate([]Persona{
		{ID: "a", Name: "A", Role: "r1"},
		{ID: "a", Name: "B", Role: "r2"},
	})
	if err == nil {
		t.Fatal("expected duplicate id error")
	}
}
