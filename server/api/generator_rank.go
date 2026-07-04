package api

// generatorRank ranks the currently-servable generator versions; selection
// prefers the highest-ranked version present among candidates. A new generator
// version is added here (see docs/generator-versions.md). Every older version
// is retired — migration 47 marks its rows status='deprecated', so selection
// never sees them — and is intentionally absent here: an unranked/legacy string
// maps to 0, below every ranked version.
var generatorRank = map[string]int{
	"heuristic_2.0": 1,
	"llm_0.6":       2,
}
