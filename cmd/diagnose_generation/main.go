// diagnose_generation runs the real LLM generator for a fixed envelope and
// target difficulty, then reports the distribution of COMPUTED difficulties of
// what it produced, the admission/envelope outcome of each candidate, how many
// land in the selection window, and - for word problems - whether the LLM
// emitted a valid symbolic_expression. It writes nothing - pure in-memory
// compute on top of the real OpenAI call.
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
	"garydmenezes.com/mathgame/server/mathcore"
)

func main() {
	configPath := flag.String("config", "conf.json", "path to config JSON (needs a live openai_api_key)")
	envelope := flag.Uint64("bitmap", 968, "problem_type_bitmap (the user's envelope)")
	target := flag.Float64("target", 20.32, "target difficulty")
	epsilon := flag.Float64("epsilon", 1.5, "selection window half-width (problemSelectionEpsilon)")
	n := flag.Int("n", 50, "number of candidates to request from the LLM")
	model := flag.String("model", "", "OpenAI model id override, e.g. gpt-5-mini or gpt-5; empty uses the generator default (gpt-5-nano)")
	flag.Parse()

	c, err := common.ReadConfig(*configPath)
	if err != nil {
		glog.Fatalf("read config: %v", err)
	}
	if err := c.Validate(); err != nil {
		glog.Fatalf("invalid config: %v", err)
	}

	pt := mathcore.ProblemType(*envelope)
	opts := &llm_generator.Options{
		Features:         mathcore.ProblemTypeToFeatures(pt),
		TargetDifficulty: *target,
		NumProblems:      *n,
		Constraints:      mathcore.BuildBitConstraints(pt),
		Model:            *model,
	}

	modelLabel := *model
	if modelLabel == "" {
		modelLabel = "gpt-5-nano (default)"
	}
	lo, hi := *target-*epsilon, *target+*epsilon
	fmt.Printf("Requesting %d candidates for bitmap=%d target=%.2f (window [%.2f, %.2f]) model=%s\n",
		*n, *envelope, *target, lo, hi, modelLabel)

	problems, err := llm_generator.GenerateProblem(opts)
	if err != nil {
		glog.Fatalf("GenerateProblem: %v", err)
	}
	fmt.Printf("LLM returned %d candidates.\n\n", len(problems))

	admitRejects := map[string]int{}
	var diffs []float64
	admitted, envelopePass, inWindow, inWindowAndEnvelope := 0, 0, 0, 0
	wordCount, formOK, formMissing, formInvalid := 0, 0, 0, 0
	histogram := map[int]int{}

	fmt.Printf("  %-7s %-9s %-7s %-8s %s\n", "diff", "envelope", "window", "form", "expression")
	for _, p := range problems {
		adm := mathcore.AdmitExpression(p.Expression)
		if adm.RejectStage != "" {
			admitRejects[adm.RejectStage]++
			continue
		}
		admitted++
		bits := mathcore.NormalizeProblemBitmap(adm.Bitmap)
		inEnvelope := bits != 0 && bits&^*envelope == 0

		// For WORD problems, flag whether the LLM emitted a valid
		// symbolic_expression (the form difficulty is scored from).
		isWord := bits&uint64(mathcore.WORD) != 0
		form := "-"
		if isWord {
			wordCount++
			switch {
			case p.SymbolicExpression == "":
				form, formMissing = "MISSING", formMissing+1
			case api.VerifyAnswer(p.SymbolicExpression, p.Answer) != nil:
				form, formInvalid = "INVALID", formInvalid+1
			default:
				form, formOK = "ok", formOK+1
			}
		}

		diff := mathcore.ComputeProblemDifficulty(adm.Expr, p.SymbolicExpression)
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
		// For a WORD problem, the full prose AND the symbolic form it's scored
		// from, so the form's fidelity to the problem is eyeballable.
		shown := adm.Expr
		if isWord && p.SymbolicExpression != "" {
			shown = "⟨" + p.SymbolicExpression + "⟩ " + adm.Expr
		}
		fmt.Printf("  %-7.2f %-9s %-7s %-8s %s\n", diff, env, win, form, shown)
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
	if wordCount > 0 {
		fmt.Printf("  word problems: %d (of %d admitted; %d symbolic)\n", wordCount, admitted, admitted-wordCount)
		fmt.Printf("    valid form:   %d/%d (%.0f%%)\n", formOK, wordCount, 100*float64(formOK)/float64(wordCount))
		fmt.Printf("    missing form: %d\n", formMissing)
		fmt.Printf("    invalid form: %d (rejected at generation)\n", formInvalid)
	}
}
