package llm

import (
	"testing"
)

func TestExtractJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantMode string
	}{
		{
			name:     "fenced json",
			input:    "```json\n{\"mode\": \"answer\", \"answer\": \"hello\"}\n```",
			wantMode: "answer",
		},
		{
			name:     "bare json",
			input:    "{\"mode\": \"script\", \"language\": \"python\", \"script\": \"print(1)\"}",
			wantMode: "script",
		},
		{
			name:     "json surrounded by prose",
			input:    "Here is your answer:\n{\"mode\": \"answer\", \"answer\": \"test\"}\nHope this helps!",
			wantMode: "answer",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := extractJSON(tt.input)
			if g == nil {
				t.Fatalf("extractJSON returned nil for %s", tt.name)
			}
			if g.Mode != tt.wantMode {
				t.Errorf("got mode %q, want %q", g.Mode, tt.wantMode)
			}
		})
	}
}

func TestNormalize(t *testing.T) {
	g := &Generation{
		Mode:   "script",
		Script: "print('hello')",
	}
	norm, err := normalize(g)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if norm.Language != "python" {
		t.Errorf("got language %q, want 'python'", norm.Language)
	}
}

func TestAgyClientInstantiation(t *testing.T) {
	client := New(ProviderAgy, "agy", "")
	if client.provider != ProviderAgy {
		t.Errorf("got provider %q, want %q", client.provider, ProviderAgy)
	}
}
