package openai

import "testing"

func TestLooksLikeTruncatedText(t *testing.T) {
	tests := []struct {
		name             string
		text             string
		completionTokens int
		maxOutputTokens  int
		want             bool
	}{
		{
			name:             "not near cap",
			text:             "정상 문장입니다",
			completionTokens: 120,
			maxOutputTokens:  320,
			want:             false,
		},
		{
			name:             "near cap and no sentence ending",
			text:             "핵심 합의는 이루어졌지만 다음 단계에서",
			completionTokens: 318,
			maxOutputTokens:  320,
			want:             true,
		},
		{
			name:             "near cap but complete punctuation",
			text:             "핵심 합의는 이루어졌습니다.",
			completionTokens: 319,
			maxOutputTokens:  320,
			want:             false,
		},
		{
			name:             "near cap but complete korean ending",
			text:             "핵심 합의는 이루어졌습니다",
			completionTokens: 319,
			maxOutputTokens:  320,
			want:             false,
		},
		{
			name:             "near cap but completes directive no",
			text:             "CLOSE: no",
			completionTokens: 320,
			maxOutputTokens:  320,
			want:             false,
		},
		{
			name:             "empty text",
			text:             "   ",
			completionTokens: 320,
			maxOutputTokens:  320,
			want:             true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := looksLikeTruncatedText(tc.text, tc.completionTokens, tc.maxOutputTokens)
			if got != tc.want {
				t.Fatalf("looksLikeTruncatedText()=%v, want=%v", got, tc.want)
			}
		})
	}
}
