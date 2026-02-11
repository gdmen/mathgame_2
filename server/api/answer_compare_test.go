package api

import (
	"testing"
)

func TestParseAnswerToRat(t *testing.T) {
	tests := []struct {
		input string
		want  string // rational string "a/b" or "a" for integers
		ok    bool
	}{
		{"1/2", "1/2", true},
		{"2/4", "1/2", true},
		{"0.5", "1/2", true},
		{".5", "1/2", true},
		{"1.5", "3/2", true},
		{"1 1/2", "3/2", true},
		{"2 3/4", "11/4", true},
		{"-1/2", "-1/2", true},
		{"-.5", "-1/2", true},
		{"-1 1/2", "-3/2", true},
		{"5", "5", true},
		{"-3", "-3", true},
		{"0", "0", true},
		{"  .25  ", "1/4", true},
		{"", "", false},
		{"abc", "", false},
		{"1/0", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			r, ok := parseAnswerToRat(tt.input)
			if ok != tt.ok {
				t.Errorf("parseAnswerToRat(%q) ok = %v, want %v", tt.input, ok, tt.ok)
				return
			}
			if !ok {
				return
			}
			got := r.RatString()
			if got != tt.want {
				t.Errorf("parseAnswerToRat(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestAnswersEquivalent(t *testing.T) {
	equivalents := [][]string{
		{"1/2", "2/4", "0.5", ".5", "1/2"},
		{"1.5", "1 1/2", "3/2", "6/4"},
		{"0", "0.0", "0/1"},
		{"-1/2", "-.5", "-2/4"},
		{"-1.5", "-1 1/2", "-3/2"},
	}
	for _, group := range equivalents {
		for i, a := range group {
			for j, b := range group {
				if i == j {
					continue
				}
				if !AnswersEquivalent(a, b) {
					t.Errorf("AnswersEquivalent(%q, %q) = false, want true", a, b)
				}
			}
		}
	}

	inequivalents := []struct{ a, b string }{
		{"1/2", "1/3"},
		{"1", "2"},
		{"0.5", "0.6"},
		{"1 1/2", "1 2/3"},
		{"abc", "1/2"},
		{"1/2", "xyz"},
	}
	for _, tt := range inequivalents {
		if AnswersEquivalent(tt.a, tt.b) {
			t.Errorf("AnswersEquivalent(%q, %q) = true, want false", tt.a, tt.b)
		}
	}
}
