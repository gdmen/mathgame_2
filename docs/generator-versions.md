# Problem Generator Versions

Every problem in the database is tagged with a `generator` string that records
which version of which generator produced it. This lets us track provenance,
compare quality across versions, and filter/audit old content when regenerating
problems.

The `generator` column is never modified after a problem is created; it's
written once at generation time and preserved for history even when new
versions ship.

## Heuristic Generator (`server/generator`)

Deterministic Go code that generates arithmetic problems in-process. No API
calls, no cost, fast. Grade-aware in 1.0+.

### `heuristic_0.0` — human alpha

The original hand-written generator. Features:
- Addition, subtraction, multiplication operations only
- Iterative problem building driven by a log₃(number) difficulty scale
- Output format wrapped single numbers in parens: `(3)+(5)-(2)`
- No grade awareness; same output for all ages
- Only wired up for add/sub at difficulty ≤ 5 (via `runHeuristicGenerator`
  check in `server/api/generate_problems.go`)
- No fraction support (code path existed but was commented out)
- Multiplication supported in the generator but not exposed by the runner

Active from project start through commit before `heuristic_1.0` landed.
Problems tagged `heuristic_0.0` remain in the DB for history.

### `heuristic_1.0` — first LLM-written version

Complete rewrite. Grade-aware, template-based, produces clean output. Written
by Claude as the first AI-authored generator version.

Changes vs `0.0`:
- Grade-aware number ranges and capabilities per Common Core progression
  (grades 1-8 in `config.go`)
- Four operations supported end-to-end: `+`, `-`, `*`, `/` (division
  guaranteed to produce whole-number results)
- Multiple problem shape templates: basic binary, missing-number
  (`? + 5 = 12`), multi-term chains (`a + b - c`), same-denominator
  fractions, different-denominator fractions
- Clean expression formatting with spaces around operators: `3 + 5`
  not `(3)+(5)`
- Distinguishes fraction slashes (`1/2`) from division operators (` / `)
- Trivial-problem guards (no `a + 0`, `a - a`, `a × 1`, etc.)
- Heuristic is now the default for non-word-problem generation, not just
  a fallback. Covers all grades and all non-word problem types.

## LLM Generator (`server/llm_generator`)

Calls OpenAI (GPT-4o-mini for generation, GPT-4o for validation). Produces
richer, more varied problems, especially word problems. Slower and costs
money per problem; offset by batching (5-20 problems per call).

### `llm_0.1` — first LLM prompt

Initial prompt template. Features:
- Generates problems via GPT-4o-mini with a single generic prompt
- Difficulty specified to the LLM as "age in years" (the LLM returns
  its own self-assessed difficulty)
- Validation via GPT-4o: checks the generated answer is correct, flags
  mismatches
- No grade-level context; all problems come from the same generic prompt
- Supports word problems, fractions, negatives, and all four basic
  operations through the `Features` bitmap

### `llm_0.2` — curriculum alignment (WS2)

Added grade-level curriculum context and few-shot examples.
Changes vs `0.1`:
- Prompt now includes grade-level context when `GradeLevel > 0`:
  Common Core strand references, grade description, and 3 few-shot
  example problems at the target grade level (from `curriculum.json`)
- Validation prompt extended to also check grade-level appropriateness
  when grade is set (still a single API call — answer correctness and
  grade alignment checked together)
- Topic-specific variety hints injected into the prompt to encourage
  diverse problem shapes (missing addend, word problems, arrays, etc.)
- Difficulty calibration: rejects problems where the LLM's self-reported
  difficulty diverges >100% from the requested target
- Added GPT5/GPT5Nano model constants (go-openai v1.41.2)

## When a problem is served

1. `generate_problems.go:selectProblem` first checks the spaced-repetition
   review queue for due problems.
2. If no review is due, it runs a topic-weighted selection from the
   existing problem pool (matching `grade_level` and `difficulty`).
3. If the pool doesn't have enough matching problems, it triggers
   background generation.
4. Generation routes to the **heuristic generator** when the enabled
   problem types don't include `WORD`, otherwise to the **LLM generator**.
5. If the LLM fails, it falls back to the heuristic generator (stripping
   `WORD` from the bitmap).

## Adding a new version

When you ship material changes to a generator, bump the version:

- Small prompt tweaks or parameter adjustments: patch bump (`0.1` → `0.2`)
- New templates, new operations, new grade configs: minor (`1.0` → `1.1`)
- Complete rewrite or incompatible output format: major (`1.0` → `2.0`)

Update the `VERSION` constant in the generator package and add an entry
here describing what changed. Old-version problems remain in the DB and
are served normally.
