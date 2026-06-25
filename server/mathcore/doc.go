// Package mathcore is the shared math kernel: expression normalization and
// lexing, exact rational evaluation, the problem-type bit inventory and
// detection, the universal difficulty formula and per-bitmap ceiling, the
// admission pipeline (minus the final DB insert), and the prompt-constraint
// builder.
//
// It depends only on the standard library (+ math/big) so that both the api
// package and the generator packages can import it without an import cycle:
// api imports the generators (generate_problems.go), so the generators cannot
// import api, and a second copy of the evaluator/formula that must mirror api
// forever is the failure mode this package exists to prevent.
//
// Part of the problem-generation system - documented in
// docs/problem-generation.md. Behavior changes here (bits, formula, pipeline,
// alphabet, masks, reject rules) REQUIRE updating that doc in the same PR.
// Formula changes also require a DifficultyVersion bump.
package mathcore
