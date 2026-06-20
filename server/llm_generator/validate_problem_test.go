package llm_generator

import (
	"errors"
	"testing"
)

func TestParseValidatorResponse(t *testing.T) {
	word := &Problem{Expression: `\text{...}`, Answer: "120"}
	form := &Problem{Expression: `\text{...}`, Answer: "120", SymbolicExpression: "60 * 2"}

	cases := []struct {
		name     string
		p        *Problem
		content  string
		wantErr  error // sentinel to errors.Is against; nil = success
		wantFeat []string
	}{
		{"3-line ok", word, "120\nYES\ndivision, word", nil, []string{"division", "word"}},
		{"answer mismatch", word, "121\nYES\ndivision", errSome, nil},
		{"envelope no", word, "120\nNO\ndivision", ErrEnvelopeMismatch, nil},
		{"form ok", form, "120\nYES\nmultiplication, word\nYES", nil, []string{"multiplication", "word"}},
		{"form mismatch", form, "120\nYES\nmultiplication, word\nNO", ErrFormMismatch, nil},
		// No form requested -> a stray 4th line is ignored.
		{"no form, extra line ignored", word, "120\nYES\nword\nNO", nil, []string{"word"}},
		// Form requested but the model omitted line 4 -> not rejected (the
		// in-code answer check still gates it).
		{"form requested, line 4 missing", form, "120\nYES\nmultiplication", nil, []string{"multiplication"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			feat, err := parseValidatorResponse(tc.content, tc.p)
			switch {
			case tc.wantErr == nil && err != nil:
				t.Fatalf("unexpected error: %v", err)
			case tc.wantErr == errSome && err == nil:
				t.Fatal("expected an error, got nil")
			case tc.wantErr != nil && tc.wantErr != errSome && !errors.Is(err, tc.wantErr):
				t.Fatalf("error = %v, want %v", err, tc.wantErr)
			}
			if tc.wantErr == nil {
				if len(feat) != len(tc.wantFeat) {
					t.Fatalf("features = %v, want %v", feat, tc.wantFeat)
				}
				for i := range feat {
					if feat[i] != tc.wantFeat[i] {
						t.Fatalf("features = %v, want %v", feat, tc.wantFeat)
					}
				}
			}
		})
	}
}

// errSome marks a case that should error without a specific sentinel.
var errSome = errors.New("some error")
