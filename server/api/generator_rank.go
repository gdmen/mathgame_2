package api

// generatorRank ranks known generator versions; selection prefers the
// highest-ranked version present among candidates. A new generator version is
// added here (see docs/generator-versions.md). An unranked/legacy string maps
// to 0, below every known version.
var generatorRank = map[string]int{
	"heuristic_0.0": 1,
	"llm_0.1":       2,
	"heuristic_1.0": 3,
	"llm_0.2":       4,
	"llm_0.3":       5,
	"llm_0.4":       6,
	"llm_0.5":       7,
	// heuristic_2.0 is difficulty-targeting and free; for the symbolic cells it
	// can fill it is preferred over the current LLM (llm_0.5), the "deterministic
	// code owns the math" direction of #283 — so it ranks above it. WORD cells are
	// unaffected: the heuristic emits no WORD rows, so the LLM remains the only
	// (and thus top-ranked) generator present there.
	"heuristic_2.0": 8,
}
