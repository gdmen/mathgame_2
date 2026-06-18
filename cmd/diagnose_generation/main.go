// diagnose_generation runs the real LLM generator for a fixed envelope and
// target difficulty, then reports the distribution of COMPUTED difficulties of
// what it produced, the admission/envelope outcome of each candidate, and how
// many land in the selection window. It writes nothing - pure in-memory compute
// on top of the real OpenAI call.
//
// It mirrors the generation path in server/api/generate_problems.go (build the
// constraint block from the bitmap, call the generator, run AdmitExpression +
// NormalizeProblemBitmap, compute the universal difficulty) without the DB
// write or the WORD-validator round-trip.
//
// Run from a directory with a real conf.json (live openai_api_key): the
// generator reads conf.json from CWD for the API key.
//
// Usage:
//
//	./diagnose_generation -config=conf.json -bitmap=968 -target=20.32 -n=50
package main

import (
	"flag"
	"fmt"
	"math"
	"sort"

	"github.com/golang/glog"

	"garydmenezes.com/mathgame/server/api"
	"garydmenezes.com/mathgame/server/common"
	"garydmenezes.com/mathgame/server/llm_generator"
)

func main() {
	configPath := flag.String("config", "conf.json", "path to config JSON (needs a live openai_api_key)")
	envelope := flag.Uint64("bitmap", 968, "problem_type_bitmap (the user's envelope)")
	target := flag.Float64("target", 20.32, "target difficulty")
	epsilon := flag.Float64("epsilon", 1.5, "selection window half-width (problemSelectionEpsilon)")
	n := flag.Int("n", 50, "number of candidates to request from the LLM")
	flag.Parse()

	c, err := common.ReadConfig(*configPath)
	if err != nil {
		glog.Fatalf("read config: %v", err)
	}
	if err := c.Validate(); err != nil {
		glog.Fatalf("invalid config: %v", err)
	}

	pt := api.ProblemType(*envelope)
	opts := &llm_generator.Options{
		Features:         api.ProblemTypeToFeatures(pt),
		TargetDifficulty: *target,
		NumProblems:      *n,
		Constraints:      api.BuildBitConstraints(pt),
	}

	lo, hi := *target-*epsilon, *target+*epsilon
	fmt.Printf("Requesting %d candidates for bitmap=%d target=%.2f (window [%.2f, %.2f])\n",
		*n, *envelope, *target, lo, hi)

	problems, err := llm_generator.GenerateProblem(opts)
	if err != nil {
		glog.Fatalf("GenerateProblem: %v", err)
	}
	fmt.Printf("LLM returned %d candidates.\n\n", len(problems))

	admitRejects := map[string]int{}
	var diffs []float64
	admitted, envelopePass, inWindow, inWindowAndEnvelope := 0, 0, 0, 0
	histogram := map[int]int{}

	fmt.Printf("  %-7s %-9s %-7s %s\n", "diff", "envelope", "window", "expression")
	for _, p := range problems {
		adm := api.AdmitExpression(p.Expression)
		if adm.RejectStage != "" {
			admitRejects[adm.RejectStage]++
			continue
		}
		admitted++
		bits := api.NormalizeProblemBitmap(adm.Bitmap)
		inEnvelope := bits != 0 && bits&^*envelope == 0

		diff := api.ComputeProblemDifficulty(adm.Expr)
		diffs = append(diffs, diff)
		histogram[int(math.Floor(diff))]++

		inWin := diff >= lo && diff <= hi
		if inEnvelope {
			envelopePass++
		}
		if inWin {
			inWindow++
		}
		if inWin && inEnvelope {
			inWindowAndEnvelope++
		}

		env := "REJECT"
		if inEnvelope {
			env = "ok"
		}
		win := "-"
		if inWin {
			win = "IN"
		}
		expr := adm.Expr
		if len(expr) > 80 {
			expr = expr[:80] + "…"
		}
		fmt.Printf("  %-7.2f %-9s %-7s %s\n", diff, env, win, expr)
	}

	fmt.Printf("\nSummary (%d returned):\n", len(problems))
	if len(admitRejects) > 0 {
		fmt.Printf("  admit rejects: ")
		for _, stage := range []string{"lexer", "unknown_rules"} {
			if admitRejects[stage] > 0 {
				fmt.Printf("%s=%d ", stage, admitRejects[stage])
			}
		}
		fmt.Println()
	}
	fmt.Printf("  admitted:      %d\n", admitted)
	fmt.Printf("  envelope pass: %d/%d admitted\n", envelopePass, admitted)
	if len(diffs) > 0 {
		sort.Float64s(diffs)
		sum := 0.0
		for _, d := range diffs {
			sum += d
		}
		fmt.Printf("  difficulty:    min %.2f, median %.2f, mean %.2f, max %.2f\n",
			diffs[0], diffs[len(diffs)/2], sum/float64(len(diffs)), diffs[len(diffs)-1])
		fmt.Printf("  distribution:")
		keys := make([]int, 0, len(histogram))
		for k := range histogram {
			keys = append(keys, k)
		}
		sort.Ints(keys)
		for _, k := range keys {
			fmt.Printf(" [%d-%d]:%d", k, k+1, histogram[k])
		}
		fmt.Println()
	}
	fmt.Printf("  in window [%.2f, %.2f]: %d/%d admitted (%d also envelope-pass)\n",
		lo, hi, inWindow, admitted, inWindowAndEnvelope)
}
