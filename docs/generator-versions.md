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

### `llm_0.3` — bitmap constraint block

The prompt is driven by the user's problem-type bitmap
(see docs/problem-generation.md). Changes vs `0.2`:
- The MAY / MUST NOT constraint block (api.BuildBitConstraints) is the
  sole shape guidance: per-bit allow/forbid pairs, magnitude clause,
  chain clause, unknown rules, closed-world clause. Grade-level
  curriculum context, few-shot curriculum examples, and the
  "age in years" difficulty framing are removed (`curriculum.json`
  deleted).
- Generator self-report is no longer trusted: features are stamped by
  the admission pipeline's detector, difficulty by
  ComputeProblemDifficulty. The self-reported-difficulty calibration
  gate is removed.
- Validation is local-first: symbolic problems are answer-checked by the
  exact evaluator with no API call; word problems get one 3-line
  validator round-trip (answer, envelope yes/no, observed features).
- Storage preserves original notation (\frac, \times render through
  KaTeX); normalization is internal to parsing.

### `llm_0.4` — self-contained word problems

Prompt-only change vs `0.3`. A prod audit (issue #249) found ~84% of
llm_0.3 word problems appended the bare computation to the prose
(`\text{...3 bags of 6 apples...}3 * 6`), and a subset appended the full
equation including the result (`...= 18`), revealing the answer. The
model was over-applying the "symbolic math goes outside \text{}" rule to
story problems. The prompt now says: write the ENTIRE word problem as
prose, never append the arithmetic or its result, and use symbolic math
outside \text{} ONLY when the statement itself is an expression to
manipulate (e.g. "Solve for x: 3x + 7 = 22"). No code path changed.

### `llm_0.5` — symbolic_expression for word problems

Issue #266. Word problems were being scored as addition: the difficulty
formula reads operators from tokens, and a story problem's operation lives in
prose inside `\text{}`, invisible to it. The generator now emits a
`symbolic_expression` alongside each word problem — the exact computation it
asks for (e.g. `9999 / 3 / 3`), prompted as the same operations and numbers the
student would use, not merely something that hits the answer. It is stored,
never shown to the student, and validated three ways: it must lex and evaluate
to the answer in-code, and the WORD validator's line 4 confirms it matches the
problem's actual operations (catching a form that reaches the answer with the
wrong computation). Difficulty is scored from the form with the word concept
applied, so a division word problem scores like its symbolic twin plus the word
bonus. Non-word problems leave it empty (their expression is already symbolic).
Carries a `DifficultyVersion` bump (0.2 → 0.3) and a
`recompute_problem_difficulty` run on deploy.

## Universal Difficulty Scale

Every problem's `difficulty` column stores a universal score on a 1-20 scale
computed from the expression itself via `api.ComputeProblemDifficulty(expr)`.
See `server/api/difficulty.go` for the formula.

The score is a log-compressed composite of:
- **magnitude** — `log10` of the largest operand
- **op_weight** — hardest operation present (1.0 add → 4.0 exponent)
- **concept** — multipliers for fractions, negatives, variables, word problems
- **structure** — chain length and missing-number bump

Rough alignment with Common Core progression:

| Scale | Typical content |
|-------|-----------------|
| 1-3   | Grade 1: counting, basic add within 20 |
| 3-5   | Grade 2: add/sub within 100, missing addend |
| 5-8   | Grade 3: multiplication facts, simple fractions |
| 8-11  | Grade 4: multi-digit mul, fraction add/sub |
| 11-14 | Grade 5: unlike-denom fractions, order of ops |
| 14-16 | Grade 6-7: negatives, proportional reasoning |
| 16-20 | Grade 8+: algebra, exponents |

Selection filters problems with `difficulty BETWEEN target*0.7 AND target*1.3`
on the universal scale. `grade_level` is still saved on each problem for
provenance and LLM prompt context but is **not** used as a pool filter. A
struggling grade 5 kid whose target drifts to 5 gets actual difficulty-5
problems (from any generator, any grade) — no more contradictory "difficulty 3
for a 5th grader" requests to the LLM.

The `grade_level` setting in user settings drives the `maxDiff` cap for the
adaptive loop (`grade*2+4`) so moving a user's grade up/down raises/lowers
their difficulty range. It's the parent's explicit lever.

To recompute legacy problem difficulties after deployment:

```
./bin/recompute_problem_difficulty -config=prod_conf.json
# or --dry-run first
```

## When a problem is served

1. `generate_problems.go:selectProblem` first checks the spaced-repetition
   review queue for due problems.
2. If no review is due, it runs a topic-weighted selection from the
   existing problem pool (matching `difficulty` within ±30% of target).
3. If the pool doesn't have enough matching problems, it triggers
   background generation.
4. Generation routes to the **heuristic generator** when the enabled
   problem types don't include `WORD`, otherwise to the **LLM generator**.
5. If the LLM fails, it falls back to the heuristic generator (stripping
   `WORD` from the bitmap).
6. When a problem is created, its `difficulty` column is set to
   `ComputeProblemDifficulty(expression)` — not the requester's target.

## Adding a new version

When you ship material changes to a generator, bump the version:

- Small prompt tweaks or parameter adjustments: patch bump (`0.1` → `0.2`)
- New templates, new operations, new grade configs: minor (`1.0` → `1.1`)
- Complete rewrite or incompatible output format: major (`1.0` → `2.0`)

Update the `VERSION` constant in the generator package and add an entry
here describing what changed. Old-version problems remain in the DB and
are served normally.
