// gen_difficulty_fixtures regenerates web/src/difficulty_band_fixtures.json:
// the shared parity fixtures pinning the JS mirror (bitmap_validation.js
// maxDiffForBitmap/minDiffForBitmap) to the Go source of truth
// (mathcore.MaxDiffForBitmap/MinDiffForBitmap).
//
// The fixture set covers every formula branch: each single bit's ceiling
// lift, the magnitude brackets, the MISSING/SINGLE_VARIABLE either-or, the
// word/non-word either-or (chain cap, PEMDAS suppression), cumulative
// multi-concept ladders, spiky profiles, and the per-op floors.
//
// Guarded from both sides: TestDifficultyBandFixturesSync (server/api) fails
// when the fixtures no longer match the Go formula (regenerate with
// `make gen-difficulty-fixtures`), and the bitmap_validation.test.js fixture
// loop fails when the JS mirror no longer matches the fixtures.
//
// Usage: go run ./cmd/gen_difficulty_fixtures (from the repo root; the
// Makefile target does this).
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"garydmenezes.com/mathgame/server/mathcore"
)

type fixture struct {
	Name string  `json:"name"`
	Bits uint64  `json:"bits"`
	Max  float64 `json:"max"`
	Min  float64 `json:"min"`
	Lo   float64 `json:"lo"`
	Hi   float64 `json:"hi"`
}

func main() {
	base := mathcore.ADDITION | mathcore.SUBTRACTION
	p1 := base
	p2 := p1 | mathcore.MEDIUM_NUMBERS | mathcore.MISSING_NUMBER
	p3 := p2 | mathcore.MULTIPLICATION | mathcore.DIVISION
	p4 := p3 | mathcore.LARGE_NUMBERS | mathcore.CHAINED_OPERATIONS
	p5 := p4 | mathcore.FRACTIONS | mathcore.WORD
	p6 := p5 | mathcore.MISMATCHED_DENOMINATORS | mathcore.DECIMALS
	p7 := p6 | mathcore.NEGATIVES | mathcore.PEMDAS | mathcore.PERCENTAGES
	p8 := p7 | mathcore.SINGLE_VARIABLE

	type namedBits struct {
		name string
		bits mathcore.ProblemType
	}
	var cases []namedBits

	// Every single bit alone and on the ADD|SUB baseline - derived from the
	// bit inventory, so a new bit is covered without touching this tool.
	for bit := mathcore.ProblemType(1); bit <= mathcore.ALL_PROBLEM_TYPES; bit <<= 1 {
		if bit&mathcore.ALL_PROBLEM_TYPES == 0 {
			continue
		}
		name := mathcore.ProblemTypeToFeatures(bit)[0]
		cases = append(cases,
			namedBits{name, bit},
			namedBits{"base+" + name, base | bit},
		)
	}

	cases = append(cases,
		// Word/non-word either-or branches.
		namedBits{"word+chained (word chain cap)", base | mathcore.WORD | mathcore.CHAINED_OPERATIONS},
		namedBits{"word+chained+pemdas (non-word branch wins)", base | mathcore.WORD | mathcore.CHAINED_OPERATIONS | mathcore.PEMDAS},
		namedBits{"word+chained+missing", base | mathcore.WORD | mathcore.CHAINED_OPERATIONS | mathcore.MISSING_NUMBER},
		namedBits{"word+chained+variable", base | mathcore.WORD | mathcore.CHAINED_OPERATIONS | mathcore.SINGLE_VARIABLE},
		namedBits{"field case: 4ops+neg+word+medium+chained", mathcore.ADDITION | mathcore.SUBTRACTION |
			mathcore.MULTIPLICATION | mathcore.DIVISION | mathcore.NEGATIVES | mathcore.WORD |
			mathcore.MEDIUM_NUMBERS | mathcore.CHAINED_OPERATIONS},
		// High-floor combos.
		namedBits{"div+mul", mathcore.DIVISION | mathcore.MULTIPLICATION},
		// Cumulative ladder and spiky profiles.
		namedBits{"p2", p2},
		namedBits{"p3", p3},
		namedBits{"p4", p4},
		namedBits{"p5", p5},
		namedBits{"p6", p6},
		namedBits{"p7", p7},
		namedBits{"p8 (everything)", p8},
		namedBits{"spiky add+medium+chained", mathcore.ADDITION | mathcore.MEDIUM_NUMBERS | mathcore.CHAINED_OPERATIONS},
		namedBits{"spiky frac+mismatch+medium", mathcore.ADDITION | mathcore.SUBTRACTION | mathcore.FRACTIONS |
			mathcore.MISMATCHED_DENOMINATORS | mathcore.MEDIUM_NUMBERS},
		namedBits{"spiky add+medium+decimals", mathcore.ADDITION | mathcore.MEDIUM_NUMBERS | mathcore.DECIMALS},
	)

	var fixtures []fixture
	for _, c := range cases {
		bits := uint64(c.bits)
		lo, hi := mathcore.TargetDifficultyRange(bits)
		fixtures = append(fixtures, fixture{
			Name: c.name,
			Bits: bits,
			Max:  mathcore.MaxDiffForBitmap(bits),
			Min:  mathcore.MinDiffForBitmap(bits),
			Lo:   lo,
			Hi:   hi,
		})
	}

	out, err := json.MarshalIndent(fixtures, "", "  ")
	if err != nil {
		panic(err)
	}
	out = append(out, '\n')
	const path = "web/src/difficulty_band_fixtures.json"
	if err := os.WriteFile(path, out, 0644); err != nil {
		panic(err)
	}
	fmt.Printf("wrote %d fixtures to %s\n", len(fixtures), path)
}
