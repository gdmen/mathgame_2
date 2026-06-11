package api

import (
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// TestDocsSync is the mechanical layer of the doc-drift defense: it
// fails CI when docs/problem-generation.md and the code disagree on three
// anchors, so a new bit or a formula-version bump cannot land undocumented.
//
// Deliberately slim - three assertions, one trivially-parseable anchor
// block. The formula's NUMBERS are owned by ordinary unit tests
// (TestComputeProblemDifficulty_ReferenceValues); the doc's prose tables are
// illustrative. This test is a forcing function, not a correctness proof:
// it drags you into the doc at exactly the right moment, and once there,
// updating the adjacent prose is the natural completion.
func TestDocsSync(t *testing.T) {
	data, err := os.ReadFile("../../docs/problem-generation.md")
	if err != nil {
		t.Fatalf("docs/problem-generation.md unreadable: %v - the problem-generation system must stay documented", err)
	}
	doc := string(data)

	start := strings.Index(doc, "<!-- BEGIN DOC-SYNC ANCHORS")
	end := strings.Index(doc, "<!-- END DOC-SYNC ANCHORS")
	if start < 0 || end < 0 || end < start {
		t.Fatal("doc-sync anchor block missing from docs/problem-generation.md")
	}
	anchors := map[string]string{}
	for _, line := range strings.Split(doc[start:end], "\n") {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 {
			anchors[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}

	// Anchor 1: DifficultyVersion - any formula bump forces a doc touch.
	if anchors["difficulty_version"] != DifficultyVersion {
		t.Errorf("doc difficulty_version = %q, code DifficultyVersion = %q - update docs/problem-generation.md (formula change? remember the recompute deploy step)",
			anchors["difficulty_version"], DifficultyVersion)
	}

	// Anchor 2: the bit inventory - a new bit cannot land undocumented.
	var wantBits []string
	for _, name := range problemTypeNames {
		wantBits = append(wantBits, name)
	}
	sort.Strings(wantBits)
	var docBits []string
	for _, b := range strings.Split(anchors["bits"], ",") {
		if s := strings.TrimSpace(b); s != "" {
			docBits = append(docBits, s)
		}
	}
	sort.Strings(docBits)
	if strings.Join(wantBits, ",") != strings.Join(docBits, ",") {
		t.Errorf("doc bit inventory differs from problemTypeNames -\n  doc:  %v\n  code: %v\nupdate docs/problem-generation.md (new bit? walk the new-bit checklist)",
			docBits, wantBits)
	}

	// Anchor 3: the shared shape constants (generator mapping + ceiling).
	if v, _ := strconv.Atoi(anchors["max_chain_len"]); v != MaxChainLen {
		t.Errorf("doc max_chain_len = %q, code MaxChainLen = %d - update docs/problem-generation.md",
			anchors["max_chain_len"], MaxChainLen)
	}
	if v, _ := strconv.Atoi(anchors["large_max_operand"]); v != LargeMaxOperand {
		t.Errorf("doc large_max_operand = %q, code LargeMaxOperand = %d - update docs/problem-generation.md",
			anchors["large_max_operand"], LargeMaxOperand)
	}
}
