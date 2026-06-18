package api

// generatorRank ranks known generator versions; selection prefers the
// highest-ranked version present among candidates. A new generator version is
// added here (see docs/generator-versions.md). An unranked/legacy string maps
// to 0, below every known version.
var generatorRank = map[string]int{
	"llm_0.0":       1,
	"heuristic_0.0": 2,
	"llm_0.1":       3,
	"heuristic_1.0": 4,
	"llm_0.2":       5,
	"llm_0.3":       6,
	"llm_0.4":       7,
	"llm_0.5":       8,
}
