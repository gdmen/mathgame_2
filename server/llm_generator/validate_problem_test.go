package llm_generator

import (
	"errors"
	"testing"
)

func TestParseValidatorResponse(t *testing.T) {
	p := &Problem{Expression: `\text{...}`, Answer: "120", SymbolicExpression: "60 * 2"}

	cases := []struct {
		name    string
		content string
		wantErr error // sentinel to errors.Is against; nil = success
	}{
		{"answer + form ok", "120\nYES", nil},
		{"lowercase yes ok", "120\nyes", nil},
		{"answer mismatch", "121\nYES", errSome},
		{"form mismatch", "120\nNO", ErrFormMismatch},
		{"too few lines", "120", errSome},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := parseValidatorResponse(tc.content, p)
			switch {
			case tc.wantErr == nil && err != nil:
				t.Fatalf("unexpected error: %v", err)
			case tc.wantErr == errSome && err == nil:
				t.Fatal("expected an error, got nil")
			case tc.wantErr != nil && tc.wantErr != errSome && !errors.Is(err, tc.wantErr):
				t.Fatalf("error = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

// errSome marks a case that should error without a specific sentinel.
var errSome = errors.New("some error")
