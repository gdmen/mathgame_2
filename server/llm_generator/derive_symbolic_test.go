package llm_generator

import "testing"

func TestCleanDerivedForm(t *testing.T) {
	cases := map[string]string{
		"60 * 2":              "60 * 2",
		"`9999 / 3 / 3`":      "9999 / 3 / 3",
		"  72 / 8  ":          "72 / 8",
		"60 * 2\nexplanation": "60 * 2",
		"\"48 / 4\"":          "48 / 4",
		"":                    "",
	}
	for in, want := range cases {
		if got := cleanDerivedForm(in); got != want {
			t.Errorf("cleanDerivedForm(%q) = %q, want %q", in, got, want)
		}
	}
}
