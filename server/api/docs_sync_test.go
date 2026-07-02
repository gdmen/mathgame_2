package api

import (
	"encoding/json"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"

	"garydmenezes.com/mathgame/server/generator"
	"garydmenezes.com/mathgame/server/llm_generator"
	"garydmenezes.com/mathgame/server/mathcore"
)

// The DocsSync tests are the mechanical layer of the doc-drift defense: each
// fails CI when an area's source-of-truth doc and the code disagree on a small
// set of load-bearing anchors, so a constant bump, a new bit, or a new
// migration cannot land undocumented. They are forcing functions, not
// correctness proofs: the formula's NUMBERS are owned by ordinary unit tests
// and the docs' prose tables are illustrative. The anchors drag you into the
// doc at exactly the right moment; updating the adjacent prose is the natural
// completion. Run them all with `go test ./server/api/ -run DocsSync`.

// readDocAnchors parses the `key: value` lines of a doc's DOC-SYNC anchor block.
func readDocAnchors(t *testing.T, path string) map[string]string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%s unreadable: %v - this area must stay documented", path, err)
	}
	doc := string(data)
	start := strings.Index(doc, "<!-- BEGIN DOC-SYNC ANCHORS")
	end := strings.Index(doc, "<!-- END DOC-SYNC ANCHORS")
	if start < 0 || end < 0 || end < start {
		t.Fatalf("doc-sync anchor block missing from %s", path)
	}
	anchors := map[string]string{}
	for _, line := range strings.Split(doc[start:end], "\n") {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 {
			anchors[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}
	return anchors
}

// sortedCSV normalizes a comma-separated anchor value into a sorted, trimmed
// list so set comparisons ignore ordering and whitespace.
func sortedCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	sort.Strings(out)
	return out
}

// readFileForTest returns a file's contents, failing the test if unreadable.
func readFileForTest(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%s unreadable: %v", path, err)
	}
	return string(data)
}

func assertIntAnchor(t *testing.T, doc, key, value string, want int) {
	t.Helper()
	if v, _ := strconv.Atoi(value); v != want {
		t.Errorf("%s %s = %q, code = %d - update %s", doc, key, value, want, doc)
	}
}

func assertFloatAnchor(t *testing.T, doc, key, value string, want float64) {
	t.Helper()
	if v, _ := strconv.ParseFloat(value, 64); v != want {
		t.Errorf("%s %s = %q, code = %v - update %s", doc, key, value, want, doc)
	}
}

func assertSetAnchor(t *testing.T, doc, key, value string, want []string) {
	t.Helper()
	got := sortedCSV(value)
	sort.Strings(want)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("%s %s differs from code -\n  doc:  %v\n  code: %v\nupdate %s",
			doc, key, got, want, doc)
	}
}

// TestDocsSync pins docs/problem-generation.md to the difficulty version, the
// bit inventory, and the shared shape constants.
func TestDocsSync(t *testing.T) {
	const doc = "../../docs/problem-generation.md"
	anchors := readDocAnchors(t, doc)

	// Anchor 1: DifficultyVersion - any formula bump forces a doc touch.
	if anchors["difficulty_version"] != mathcore.DifficultyVersion {
		t.Errorf("doc difficulty_version = %q, code DifficultyVersion = %q - update docs/problem-generation.md (formula change? remember the recompute deploy step)",
			anchors["difficulty_version"], mathcore.DifficultyVersion)
	}

	// Anchor 2: the bit inventory - a new bit cannot land undocumented.
	wantBits := mathcore.ProblemTypeToFeatures(mathcore.ALL_PROBLEM_TYPES)
	assertSetAnchor(t, doc, "bits", anchors["bits"], wantBits)

	// Anchor 3: the shared shape constants (generator mapping + ceiling).
	assertIntAnchor(t, doc, "max_chain_len", anchors["max_chain_len"], mathcore.MaxChainLen)
	assertIntAnchor(t, doc, "large_max_operand", anchors["large_max_operand"], mathcore.LargeMaxOperand)
}

// TestDocsSyncGeneratorVersions pins docs/generator-versions.md to the live
// VERSION constant of each generator, so shipping a new generator version
// cannot land without an entry in the version-history doc.
func TestDocsSyncGeneratorVersions(t *testing.T) {
	const doc = "../../docs/generator-versions.md"
	anchors := readDocAnchors(t, doc)

	if anchors["heuristic_version"] != generator.VERSION {
		t.Errorf("doc heuristic_version = %q, code generator.VERSION = %q - add the new version to docs/generator-versions.md",
			anchors["heuristic_version"], generator.VERSION)
	}
	if anchors["llm_version"] != llm_generator.VERSION {
		t.Errorf("doc llm_version = %q, code llm_generator.VERSION = %q - add the new version to docs/generator-versions.md",
			anchors["llm_version"], llm_generator.VERSION)
	}
}

// TestDocsSyncSelection pins docs/selection.md to the selection-window
// constants.
func TestDocsSyncSelection(t *testing.T) {
	const doc = "../../docs/selection.md"
	anchors := readDocAnchors(t, doc)

	assertIntAnchor(t, doc, "recency_window", anchors["recency_window"], recencyWindow)
	assertFloatAnchor(t, doc, "lru_top_frac", anchors["lru_top_frac"], lruTopFrac)
	assertFloatAnchor(t, doc, "selection_epsilon", anchors["selection_epsilon"], problemSelectionEpsilon)
}

// TestDocsSyncAdaptiveDifficulty pins docs/adaptive-difficulty.md to the
// progression levers.
func TestDocsSyncAdaptiveDifficulty(t *testing.T) {
	const doc = "../../docs/adaptive-difficulty.md"
	anchors := readDocAnchors(t, doc)

	var intervals []string
	for _, n := range spacedRepIntervals {
		intervals = append(intervals, strconv.Itoa(n))
	}
	// Order is semantic for the intervals, so compare the sequence directly.
	var docIntervals []string
	for _, p := range strings.Split(anchors["spaced_rep_intervals"], ",") {
		docIntervals = append(docIntervals, strings.TrimSpace(p))
	}
	if strings.Join(intervals, ",") != strings.Join(docIntervals, ",") {
		t.Errorf("%s spaced_rep_intervals = %v, code spacedRepIntervals = %v - update %s",
			doc, docIntervals, intervals, doc)
	}

	assertIntAnchor(t, doc, "max_target", anchors["max_target"], maxTarget)
	assertFloatAnchor(t, doc, "min_target_difficulty", anchors["min_target_difficulty"], mathcore.MinTargetDifficulty)
	assertFloatAnchor(t, doc, "problem_selection_epsilon", anchors["problem_selection_epsilon"], problemSelectionEpsilon)
}

// TestDocsSyncEvents pins docs/events.md to the event-type inventory and the
// compression/stats constants.
func TestDocsSyncEvents(t *testing.T) {
	const doc = "../../docs/events.md"
	anchors := readDocAnchors(t, doc)

	// The full event-type inventory - a new event type cannot land undocumented.
	allEventTypes := []string{
		LOGGED_IN, SELECTED_PROBLEM, WORKING_ON_PROBLEM, ANSWERED_PROBLEM,
		SOLVED_PROBLEM, ERROR_PLAYING_VIDEO, WATCHING_VIDEO, DONE_WATCHING_VIDEO,
		SET_TARGET_DIFFICULTY, SET_TARGET_WORK_PERCENTAGE, SET_PROBLEM_TYPE_BITMAP,
		SET_GAMESTATE_TARGET, BAD_PROBLEM_SYSTEM, BAD_PROBLEM_USER,
	}
	assertSetAnchor(t, doc, "event_types", anchors["event_types"], allEventTypes)

	// summableEventTypes is a real map; assert against its keys.
	var summable []string
	for k := range summableEventTypes {
		summable = append(summable, k)
	}
	assertSetAnchor(t, doc, "summable_event_types", anchors["summable_event_types"], summable)

	// The stats cache counts these three types. There is no single code symbol
	// enumerating them (the accumulation lives in statistics_handlers.go switches),
	// so the doc is pinned to the named consts - changing the documented set
	// forces a doc touch.
	statsCounted := []string{SOLVED_PROBLEM, WORKING_ON_PROBLEM, WATCHING_VIDEO}
	assertSetAnchor(t, doc, "stats_counted_event_types", anchors["stats_counted_event_types"], statsCounted)

	assertIntAnchor(t, doc, "compress_max_chunk_size", anchors["compress_max_chunk_size"], maxChunkSize)
}

// TestDocsSyncVideos pins docs/videos.md to the YouTube API literals and the
// config requirement. These are string literals (no exported constants), so the
// assertions substring-match the source of truth.
func TestDocsSyncVideos(t *testing.T) {
	const doc = "../../docs/videos.md"
	anchors := readDocAnchors(t, doc)

	ytSrc := readFileForTest(t, "youtube.go")
	if !strings.Contains(ytSrc, anchors["youtube_api_host"]) {
		t.Errorf("%s youtube_api_host = %q not found in youtube.go", doc, anchors["youtube_api_host"])
	}
	if !strings.Contains(ytSrc, "maxResults="+anchors["playlist_items_page_size"]) {
		t.Errorf("%s playlist_items_page_size = %q not found as maxResults= in youtube.go", doc, anchors["playlist_items_page_size"])
	}
	if !strings.Contains(ytSrc, anchors["video_watch_url_prefix"]) {
		t.Errorf("%s video_watch_url_prefix = %q not found in youtube.go", doc, anchors["video_watch_url_prefix"])
	}

	// youtube_api_key_required: the key is required iff it is NOT in
	// optionalConfigFields (which Validate skips).
	cfgSrc := readFileForTest(t, "../common/config.go")
	optStart := strings.Index(cfgSrc, "optionalConfigFields = map[string]bool{")
	if optStart < 0 {
		t.Fatal("optionalConfigFields map not found in common/config.go")
	}
	optBlock := cfgSrc[optStart:]
	if i := strings.Index(optBlock, "}"); i >= 0 {
		optBlock = optBlock[:i]
	}
	required := !strings.Contains(optBlock, "youtube_api_key")
	if (anchors["youtube_api_key_required"] == "true") != required {
		t.Errorf("%s youtube_api_key_required = %q, but config requirement (not-optional) = %v",
			doc, anchors["youtube_api_key_required"], required)
	}
}

// TestDocsSyncSettings pins docs/settings.md to the envelope ceiling constants
// and the bitmap-validation error codes.
func TestDocsSyncSettings(t *testing.T) {
	const doc = "../../docs/settings.md"
	anchors := readDocAnchors(t, doc)

	assertFloatAnchor(t, doc, "min_target_difficulty", anchors["min_target_difficulty"], mathcore.MinTargetDifficulty)
	assertIntAnchor(t, doc, "ceiling_max_chain_len", anchors["ceiling_max_chain_len"], mathcore.MaxChainLen)
	assertIntAnchor(t, doc, "ceiling_large_max_operand", anchors["ceiling_large_max_operand"], mathcore.LargeMaxOperand)
	assertIntAnchor(t, doc, "ceiling_small_max_operand", anchors["ceiling_small_max_operand"], mathcore.SmallMaxOperand)
	assertIntAnchor(t, doc, "ceiling_medium_max_operand", anchors["ceiling_medium_max_operand"], mathcore.MediumMaxOperand)

	// The validation error codes are defined client-side in bitmap_validation.js;
	// assert each documented code appears there.
	jsSrc := readFileForTest(t, "../../web/src/bitmap_validation.js")
	for _, code := range sortedCSV(anchors["validation_error_codes"]) {
		if !strings.Contains(jsSrc, code) {
			t.Errorf("%s lists validation error code %q, but web/src/bitmap_validation.js never defines it", doc, code)
		}
	}
}

// TestDocsSyncSchema pins docs/schema.md to the latest migration number and the
// generated model-table inventory.
func TestDocsSyncSchema(t *testing.T) {
	const doc = "../../docs/schema.md"
	anchors := readDocAnchors(t, doc)

	// latest_migration: the highest-numbered file in the embedded migrations FS.
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		t.Fatalf("migrations dir unreadable: %v", err)
	}
	latest := 0
	for _, e := range entries {
		name := strings.TrimSuffix(e.Name(), ".sql")
		if n, err := strconv.Atoi(name); err == nil && n > latest {
			latest = n
		}
	}
	assertIntAnchor(t, doc, "latest_migration", anchors["latest_migration"], latest)

	// model_tables: every table declared in models.json.
	var mj struct {
		Models []struct {
			Table string `json:"table"`
		} `json:"models"`
	}
	if err := json.Unmarshal([]byte(readFileForTest(t, "models.json")), &mj); err != nil {
		t.Fatalf("models.json unparseable: %v", err)
	}
	var tables []string
	for _, m := range mj.Models {
		tables = append(tables, m.Table)
	}
	assertSetAnchor(t, doc, "model_tables", anchors["model_tables"], tables)
}
