package openai

import (
	"errors"
	"testing"
)

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

func TestLooksLikeTruncatedJSON(t *testing.T) {
	tests := []struct {
		name             string
		raw              string
		parseErr         error
		completionTokens int
		maxOutputTokens  int
		want             bool
	}{
		{
			name:             "unexpected eof parse error",
			raw:              `{"reached":true`,
			parseErr:         errors.New("failed to parse consensus JSON: unexpected end of JSON input"),
			completionTokens: 90,
			maxOutputTokens:  320,
			want:             true,
		},
		{
			name:             "near cap incomplete json object",
			raw:              `{"reached":true,"score":0.81,"summary":"ok"`,
			parseErr:         errors.New("failed to parse consensus JSON: invalid character"),
			completionTokens: 511,
			maxOutputTokens:  512,
			want:             true,
		},
		{
			name:             "near cap but complete object with schema error",
			raw:              `{"reached":true,"score":0.81,"summary":"ok","rationale":"x","open_risks":[]}`,
			parseErr:         errors.New("failed to parse consensus JSON: missing required consensus key: next_action_owner"),
			completionTokens: 511,
			maxOutputTokens:  512,
			want:             false,
		},
		{
			name:             "not near cap incomplete object is not assumed truncation",
			raw:              `{"reached":true,"score":0.81`,
			parseErr:         errors.New("failed to parse consensus JSON: invalid character"),
			completionTokens: 110,
			maxOutputTokens:  512,
			want:             false,
		},
		{
			name:             "empty raw output",
			raw:              "   ",
			parseErr:         errors.New("empty consensus output"),
			completionTokens: 512,
			maxOutputTokens:  512,
			want:             true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := looksLikeTruncatedJSON(tc.raw, tc.parseErr, tc.completionTokens, tc.maxOutputTokens)
			if got != tc.want {
				t.Fatalf("looksLikeTruncatedJSON()=%v, want=%v", got, tc.want)
			}
		})
	}
}
