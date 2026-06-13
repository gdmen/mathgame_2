# The problem-generation system

The canonical reference for how problems are generated, stamped, validated,
selected, and scored. **If you change any behavior described here, update
this document in the same PR.** Mechanical enforcement: a slim doc-sync test
(`server/api/docs_sync_test.go`) fails CI when the anchors below disagree
with the code, so a new bit or a formula-version bump cannot land
undocumented. Design history: issue #225.

<!-- BEGIN DOC-SYNC ANCHORS (parsed by server/api/docs_sync_test.go) -->
```
difficulty_version: 0.2
max_chain_len: 5
large_max_operand: 9999
bits: addition, subtraction, multiplication, division, fractions, negatives, word, medium_numbers, large_numbers, chained_operations, missing_number, mismatched_denominators, decimals, pemdas, single_variable, percentages
```
<!-- END DOC-SYNC ANCHORS -->

## The model

Each user has two controls:

- **`problem_type_bitmap`** — the *envelope*: the set of problem shapes
  that are OK for this kid. Enabled bit = the generator MAY include that
  feature; disabled bit = MUST NOT. The audience (kids on the autism
  spectrum) has spiky skill profiles, so the controls are independent
  per-skill toggles rather than a single banded level.
- **`target_difficulty`** — the adaptive lever *within* that envelope,
  bounded by the formula-derived ceiling (below).

A problem is served to a user iff its stamped bits are a **subset** of the
user's bitmap: `(problem_type_bitmap & ~enabled) = 0` (plus the difficulty
window and disabled/zero-bitmap filters).

## Bit reference

Internal constants are named for the **detectable expression feature**, not
the curriculum subject (`SINGLE_VARIABLE`, not `ALGEBRA`) — feature names map
1:1 to detection logic. UI labels translate to parent vocabulary
(`web/src/settings.js` `PROBLEM_TYPE_GROUPS`). Dependent toggles render on
their own row directly below their parent (parents with dependents sit at
the bottom of their card) and are disabled until the parent is on; disabling
the parent clears them. Validation errors render inside the card they
concern. Users compose their envelope directly.

| Bit | Fires when | Difficulty factor |
|---|---|---|
| `ADDITION` `SUBTRACTION` `MULTIPLICATION` `DIVISION` | operator token present | opWeight max: 1.0 / 1.1 / 2.2 / 2.8 |
| `FRACTIONS` | any fraction token (`3/8` unspaced; `\frac{a}{b}` normalizes to it) | ×2.0 (same denominators) |
| `MISMATCHED_DENOMINATORS` | ≥2 fractions, differing denominators; on WORD problems, validator-observed (prose fractions); forces FRACTIONS via the stamp-time invariant | ×1.5, stacks on the fractions ×2.0 → net ×3.0 |
| `NEGATIVES` | unary-minus number token | ×1.3 |
| `WORD` | `\text{...}` present | ×1.3 (stacks with SINGLE_VARIABLE) |
| `MEDIUM_NUMBERS` | maxMagnitude 13–99 (bracket) | via magnitude |
| `LARGE_NUMBERS` | maxMagnitude ≥ 100 (bracket — `1 + 999` is LARGE, not MEDIUM) | via magnitude |
| `CHAINED_OPERATIONS` | numOps ≥ 2 (`=` does not count); on WORD problems, validator-observed (multi-step prose), and OR'd in by the stamp-time invariant whenever ≥2 core-op bits or PEMDAS are set | structure 1 + 0.15·(numOps−1) |
| `MISSING_NUMBER` | a single `?` outside `\text{}` | structure +0.2 |
| `DECIMALS` | symbolic decimal token | ×2.0 |
| `PEMDAS` | the dual-evaluation rule (below) | ×1.5 |
| `SINGLE_VARIABLE` | variable letter with a coefficient (`3x`) or multiple occurrences (`x + x`); on WORD problems, validator-observed (pure-prose algebra) | ×5.0 |
| `PERCENTAGES` | symbolic `n%` token (evaluates as n/100) | ×2.0 |

Defaults (no bit needed): max operand ≤ 12, single operation.
`maxMagnitude` is **digit-based for decimals** (`0.75` counts as 75) — for
stamping, difficulty, and ceiling alike: magnitude bits mean digit
complexity, universally.

**The prose rule.** `\text{...}` contents are one opaque prose token.
Letters, `?`s, and operators inside prose can never fire structural bits
("John has **a** dog." must not stamp SINGLE_VARIABLE). One deliberate
split: magnitude/decimal/percent scanning for *difficulty* and for the
magnitude *shape bits* DOES read prose numerals (a word problem about 47
apples is a MEDIUM_NUMBERS problem); concept-bit detection never does — WORD
problems' topic bits come from the validator. Legacy WORD rows carry
preserved self-reported topic bits until re-stamped by
`cmd/revalidate_word_problems`, which replaces them with
validator-observed features.

**The lone-letter rewrite (stage 1.5).** A bare letter occurring exactly
once with no coefficient (`12 - x = 5`) carries no algebraic load and is
rewritten to `?` at insert (`12 - ? = 5`), stamping MISSING_NUMBER — one
notation for fill-in-the-blank; a MISSING-enabled kid without
SINGLE_VARIABLE never sees a letter. The moment the unknown must be referred
to more than once or operated on (`3x`, `x + x`), letter notation is doing
real work and it stays: that's SINGLE_VARIABLE.

**Per-problem unknown rules** (enforced at generation prompt, insert reject,
and ceiling computation — all three sites, always together): at most ONE
distinct unknown per problem; `?` may appear at most once (multi-`?` is
ambiguous/multi-answer); an unknown requires an equation.

**Settings-level dependency rules** (`web/src/bitmap_validation.js`,
mirrored nowhere else — API clients bypassing them degrade gracefully):
at least one core operation; LARGE ⇒ MEDIUM; MISMATCHED ⇒ FRACTIONS;
PEMDAS ⇒ CHAINED.

## The insert (admission) pipeline

`api.AdmitExpression` (server/api/stamping.go) — both generator paths and
the backfill run the same stages:

```
[0]   NORMALIZE   \times,\cdot -> *   \div -> /   \frac{a}{b} -> a/b
                  \left( \right) -> ( )   unicode −×÷ -> ascii
                  $15 -> 15 (money prefix)   15,000 -> 15000 (thousands)
[1]   LEX         allowlist alphabet; unknown token (\sqrt, ^, !, ...) ->
                  reject with position + token. Blocked by default: new
                  notation cannot enter the pool until deliberately added.
                  Prose-splice guard: a letter glued to a \text block that
                  continues a word ("14 b\text{ooks") is broken prose ->
                  reject, never a variable (it would corrupt as "?ooks").
[1.5] REWRITE     lone bare variable -> ? (also applied to the explanation)
[2]   DETECT      DetectProblemTypeBitmap from the parsed features
[2.5] REJECT      unknown rules (>1 distinct unknown, multi-?)
[3]   VALIDATE    local-first (below)
[3.5] ENVELOPE    stamped bits must be subset of the user's bitmap
[4]   INSERT      problemManager.Create
```

Storage keeps the **original notation** (`\frac{1}{2}`, `\times` render
through KaTeX); normalization is a parsing concern. Only the stage-1.5 `?`
splice mutates stored text.

Every drop is counted in a per-call funnel line (#230):
`funnel: requested= returned= lexer= unknown_rules= collision= answer= envelope= validator= create= inserted=`.

## Local-first validation

The deterministic tool is authoritative wherever it can operate:

| Problem class | Answer check | Envelope | Topic bits | LLM calls |
|---|---|---|---|---|
| Symbolic (incl. `?`/variable equations, fractions, decimals, `%`) | exact `big.Rat` evaluator | bit-subset check | parser | **zero** |
| WORD (prose) | LLM validator | LLM validator (same constraints as the generator) | validator's features line | one |

Disagreement = reject, not auto-correct. The answer check, the PEMDAS
dual-eval, and bit detection share one evaluator over one token stream, so
difficulty, bits, and answers cannot disagree about what an expression means.

**PEMDAS dual-evaluation rule:** evaluate each equation side twice — correct
(recursive descent, precedence + parens) vs naive (parens stripped, strict
left-to-right fold). PEMDAS fires iff they disagree. `(3 + 5) * 2` does NOT
fire (the parens spell out the natural order); `12 - (5 - 3)` fires with no
multiplication at all. Unknowns are bound to fixed rational probes — the
formula stays a pure function of the expression because the recompute
fast-path depends on that.

## Difficulty formula (v0.2) and ceiling

`ComputeProblemDifficulty` (server/api/difficulty.go):

```
magnitude = log10(maxMagnitude + 1) + 0.3      (digit-based for decimals)
opWeight  = max over present ops (table above)
concept   = product of enabled concept multipliers (table above)
structure = 1 + 0.15*(numOps - 1), +0.2 if missing-number
raw       = magnitude * opWeight * concept * structure
scaled    = 1 + 19 * (ln(raw+1) - ln(1.5)) / (ln(16) - ln(1.5))
```

Open-ended scale: floored at 1.0, **no upper clamp** (inputs are bounded by
construction; system max ≈ 62). 1–20 is the band for one/two-concept
problems; scores above 20 mean multi-concept stacks. Illustrative anchors:
`3 + 5` ≈ 3.6 · `47 + 28` ≈ 6.5 · `9 × 12` ≈ 9.1 · `3x + 7 = 22` ≈ 15.7.
**The canonical numbers live in
`TestComputeProblemDifficulty_ReferenceValues`** (difficulty_v2_test.go) —
that test owns them; this table is prose.

Changing the formula in ANY way requires bumping `DifficultyVersion` and
running `recompute_problem_difficulty` on deploy. Calibration: #35.

**The ceiling, `MaxDiffForBitmap`** — the difficulty of the hardest problem
the enabled bits can express. WHY IT EXISTS: adaptive difficulty ratchets
`target_difficulty` upward on success; without the ceiling the target drifts
above anything the envelope can produce — into a band that is empty BY
CONSTRUCTION — and selection's ±1.5 window never matches again (permanent
fallback churn). Wired at: the `process_events.go` adjuster, the
SET_TARGET_DIFFICULTY validation, the settings-PUT clamp, and the UI slider
max (`web/src/bitmap_validation.js` mirrors it for display only — the server
is authoritative). **Either/or rule:** MISSING_NUMBER and SINGLE_VARIABLE are
per-problem mutually exclusive, so the ceiling computes both branches and
takes the higher — multiplying both in would claim an unreachable ceiling and
recreate the exact drift the ceiling prevents.

Shared shape constants (generator mapping AND ceiling, lockstep):
`MaxChainLen = 5`, `LargeMaxOperand = 9999`.

## Selection

- Bitwise-subset SQL in `getSatisfyingProblemIds`,
  `getSatisfyingProblemIdsForTopic` (+ `(bitmap & topic) != 0`), and
  `getDueReviewProblem`. Zero-bitmap rows are excluded defensively (a zero
  bitmap is a subset of everything).
- Index: `(disabled, difficulty, problem_type_bitmap)` — the trailing bitmap
  column makes the subset filter covering (~20ms vs ~130ms measured at 320K
  rows; plans in the comment block of `migrations/39.sql`).
- **`WEIGHTED_TOPIC_MASK`** = all bits except MEDIUM/LARGE_NUMBERS. Gates
  `chooseWeightedTopic`, `recordTopicAttempt`, `initTopicStats`: a bit is a
  practice topic iff per-topic difficulty coheres for it; magnitude IS
  difficulty, so "weak at LARGE_NUMBERS → serve large numbers, easier"
  fights itself. Size progression is target_difficulty's job.
- **Pool-supply weighting** (server/api/pool_supply.go): thin-pool topics
  get up to 4× lottery weight (5-min cached per-bit counts), so
  hard-to-generate bits stay in rotation by weight, not by force. Both
  lottery signals - per-kid skill (demand, topic_stats.go) and pool
  supply - act at serving time; a picked-but-thin topic also triggers
  background generation.

## Generation

- **Prompt** (`api.BuildBitConstraints`): per-bit MAY/MUST NOT pairs, a
  3-state magnitude clause, a 2-state chain clause, the unknown rules
  whenever MISSING/SINGLE_VARIABLE is enabled, and the closed-world clause
  ("use ONLY what is explicitly allowed — no square roots, exponents, ...").
  All constraints are simultaneous.
  Every constraint the insert pipeline enforces must also be communicated
  here, or the generator wastes output on shapes that always reject.
- **Heuristic generator** (server/generator): bit-driven `Options`
  (MaxOperand from magnitude bits, AllowMissing/AllowMultiOp/MaxChainLen/
  SameDenomOnly from concept bits) + an expression-wide magnitude guard
  (missing-number templates embed computed values). DECIMALS/PEMDAS/
  PERCENTAGES/SINGLE_VARIABLE generation is LLM-only for now (#227).
- **WORD validator** (`llm_generator.ValidateWordProblem`): 3 lines —
  answer / envelope YES-NO (judged against the same constraints the
  generator saw; `ENVELOPE_MISMATCH`) / observed features (closed name
  list, stamps the WORD problem's topic bits — generator self-report is
  never trusted, #224).

## Backfill tools and deployment

Single server, single DB. Deploy order matters:

1. Stop the server; deploy the new binary (not serving).
2. Migrations run on startup — but run the backfills BEFORE starting:
3. `recompute_problem_type_bitmap` — restamps every row via the admission
   pipeline. SET semantics (re-runnable). WORD rows keep their legacy topic
   bits OR'd in. Lone-letter rows get the `?` splice in expression +
   explanation (+ answer), listed for spot-checking. Reports: lexer token
   census (out-of-alphabet rows), zero-bitmap review list, unknown-rule
   review list. Run with `-dry-run` first and read the census.
4. `recompute_problem_difficulty` — restamps difficulty (the version bump
   forces every row). MUST run after the bitmap tool (the rewrite mutates
   expressions).
5. Start the server.

`revalidate_word_problems` (optional, costs one LLM call per WORD row):
re-stamps WORD rows' topic bits from the validator's observed features,
replacing preserved legacy self-report. Bitmap-only writes; answer
mismatches and constraint NOs are reported and left unchanged. Run any
time after the bitmap backfill; `-dry-run`/`-limit` to sample first.

## The new-bit checklist

Every future bit (#228 EXPONENTS is the first consumer; roadmap in #231)
walks these touchpoints. The blocked-by-default design (closed-world prompt +
lexer allowlist) protects the system between additions — a new concept is
forbidden until deliberately added.

1. Constant in `server/api/enums.go` + `problemTypeNames` entry — feature-named, not subject-named.
2. Frontend constant in `web/src/enums.js`.
3. Lexer token(s) for new notation (`server/api/expression.go` — the alphabet is the single source of truth).
4. Normalizer synonyms for LaTeX/unicode dialect forms.
5. `parseProblemFeatures` field + detection logic (token-level; the prose rule applies).
6. `DetectProblemTypeBitmap` mapping line.
7. Validation-tier decision: extend the evaluator? per-bit deterministic verifier? or WORD-class LLM validation?
8. Difficulty factor + reference values in `TestComputeProblemDifficulty_ReferenceValues` + **DifficultyVersion bump** + recompute on deploy.
9. Reachability rules: per-problem exclusivity with existing bits? → prompt clause + insert reject + ceiling either/or entry (all three sites, always together).
10. `BuildBitConstraints` MAY/MUST-NOT pair.
11. `WEIGHTED_TOPIC_MASK` membership (test: does per-topic difficulty cohere?).
12. Settings dependency rules + `web/src/bitmap_validation.js` error code if needed.
13. UI: group placement (verb / noun-kind / noun-size / framing), label, helper text — against `/style-guide`.
14. Heuristic generator support, or explicit LLM-only deferral (#227-style issue).
15. Backfill: do legacy rows need re-stamping? (re-run `recompute_problem_type_bitmap`.)
16. Update THIS DOCUMENT — the doc-sync test fails CI if you skip the anchors.
