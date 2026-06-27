# The problem-generation system

The canonical reference for how problems are generated, stamped, validated,
selected, and scored. **If you change any behavior described here, update
this document in the same PR.** Mechanical enforcement: a slim doc-sync test
(`server/api/docs_sync_test.go` `TestDocsSync`) fails CI when the anchors below
disagree with the code, so a new bit or a formula-version bump cannot land
undocumented. Generator version strings (`heuristic_*`, `llm_*`) and the
`DifficultyVersion` history live in the sibling doc
[generator-versions.md](generator-versions.md) — point there, don't inline
them. Design history: issue #225.

<!-- BEGIN DOC-SYNC ANCHORS (parsed by server/api/docs_sync_test.go) -->
```
difficulty_version: 0.3
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
user's bitmap (plus the difficulty window and disabled/zero-bitmap filters).

The math kernel — lexer, evaluator, bit inventory + detection, difficulty
formula + ceiling, and the admission pipeline (minus the DB insert) — lives in
the leaf package `server/mathcore`, which both `api` and the generator packages
import (it imports neither, breaking the cycle that would otherwise force a
second evaluator). The bit enum, its 1:1 name map, and `ALL_PROBLEM_TYPES` live
in `server/mathcore/problem_type.go` (`ProblemType` iota block, `problemTypeNames`);
the inventory is pinned by the `bits` anchor above.

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
| `ADDITION` `SUBTRACTION` `MULTIPLICATION` `DIVISION` | operator token present | `opWeight` = MAX over present ops (`WeightSub`/`WeightMul`/`WeightDiv`; add is the 1.0 baseline) |
| `FRACTIONS` | any fraction token (`3/8` unspaced; `\frac{a}{b}` normalizes to it) | `ConceptFractions` (same denominators) |
| `MISMATCHED_DENOMINATORS` | ≥2 fractions, differing denominators; on WORD problems, validator-observed (prose fractions); forces FRACTIONS via the stamp-time invariant | `ConceptMismatched`, multiplied ON TOP of `ConceptFractions` |
| `NEGATIVES` | unary-minus number token | `ConceptNegatives` |
| `WORD` | `\text{...}` present | `ConceptWord` (stacks with SINGLE_VARIABLE) |
| `MEDIUM_NUMBERS` | maxMagnitude 13–99 (bracket) | via magnitude |
| `LARGE_NUMBERS` | maxMagnitude ≥ 100 (bracket — `1 + 999` is LARGE, not MEDIUM) | via magnitude |
| `CHAINED_OPERATIONS` | numOps ≥ 2 (`=` does not count); on WORD problems, validator-observed (multi-step prose), and OR'd in by the stamp-time invariant whenever ≥2 core-op bits or PEMDAS are set | structure `+StructurePerExtraOp` per op beyond the first |
| `MISSING_NUMBER` | a single `?` outside `\text{}` | structure `+StructureMissing` |
| `DECIMALS` | symbolic decimal token | `ConceptDecimals` |
| `PEMDAS` | the dual-evaluation rule (below) | `ConceptPEMDAS` |
| `SINGLE_VARIABLE` | variable letter with a coefficient (`3x`) or multiple occurrences (`x + x`); on WORD problems, validator-observed (pure-prose algebra) | `ConceptVariable` |
| `PERCENTAGES` | symbolic `n%` token (evaluates as n/100) | `ConceptPercent` |

The factor constants (`Concept*`/`Weight*`/`Structure*`) live in
`server/mathcore/difficulty.go`; their numeric values are owned by
`TestComputeProblemDifficulty_ReferenceValues` (difficulty_test.go), not this
doc. Magnitude brackets are `SmallMaxOperand` (≤12, default) /
`MediumMaxOperand` (99) / `LargeMaxOperand` (9999) in the same file. Defaults
(no bit needed): max operand ≤ 12, single operation. `maxMagnitude` is
**digit-based for decimals** (`0.75` counts as 75) — for stamping, difficulty,
and ceiling alike: magnitude bits mean digit complexity, universally
(`lexNumber`'s `DigitMagnitude`, `expression.go`).

**The prose rule.** `\text{...}` contents are one opaque prose token
(`TokText`, `expression.go`). Letters, `?`s, and operators inside prose can
never fire structural bits ("John has **a** dog." must not stamp
SINGLE_VARIABLE). One deliberate split: magnitude/decimal/percent scanning
for *difficulty* and for the magnitude *shape bits* DOES read prose numerals
(`parseProblemFeatures`'s `TokText` branch + `reProseNumber`, `difficulty.go`)
— a word problem about 47 apples is a MEDIUM_NUMBERS problem; concept-bit
detection never does — WORD problems' topic bits come from the validator.
Legacy WORD rows carry preserved self-reported topic bits until re-stamped by
`cmd/revalidate_word_problems`, which replaces them with validator-observed
features.

**The lone-letter rewrite (stage 1.5).** A bare letter occurring exactly
once with no coefficient (`12 - x = 5`) carries no algebraic load and is
rewritten to `?` at insert (`12 - ? = 5`), stamping MISSING_NUMBER — one
notation for fill-in-the-blank; a MISSING-enabled kid without
SINGLE_VARIABLE never sees a letter (`RewriteLoneVariable`, `expression.go`).
The moment the unknown must be referred to more than once or operated on
(`3x`, `x + x`), letter notation is doing real work and it stays: that's
SINGLE_VARIABLE.

**Per-problem unknown rules** (enforced at generation prompt, insert reject,
and ceiling computation — all three sites, always together): at most ONE
distinct unknown per problem; `?` may appear at most once (multi-`?` is
ambiguous/multi-answer); an unknown requires an equation
(`CountDistinctUnknowns`, `expression.go`; `VerifyAnswerSymbolic`,
`stamping.go`).

**Settings-level dependency rules** (`web/src/bitmap_validation.js`,
mirrored nowhere else — API clients bypassing them degrade gracefully):
at least one core operation; LARGE ⇒ MEDIUM; MISMATCHED ⇒ FRACTIONS;
PEMDAS ⇒ CHAINED.

## The insert (admission) pipeline

`mathcore.AdmitExpression` (server/mathcore/stamping.go) — both generator paths
and the backfill run the same stages:

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

`NormalizeExpression`/`LexExpression`/`RewriteLoneVariable` live in
`mathcore/expression.go`; `DetectProblemTypeBitmap` and the answer ([3]) and
envelope ([3.5]) checks (`VerifyAnswerSymbolic`, `EnvelopeViolation`) in
`mathcore/stamping.go`.

**Slash convention (fraction vs. division).** The lexer disambiguates the two
meanings of `/` by spacing: an *unspaced* slash is a fraction literal (`3/8`), a
*spaced* slash is the division operator (`3 / 8`); NORMALIZE folds `\frac{a}{b}`
into the unspaced form. `mathcore.Render` emits operators spaced and fraction
literals unspaced, so any rendered expression — including a fraction or decimal
operand under `*` or `/` (`3/4 / 2/3`, `6 / 3/4`, `0.2 * 3`) — lexes
unambiguously. `Render` is also **faithful**: it parenthesizes an operand
whenever infix precedence/associativity would otherwise reparse it
(`(a + b) * c`, `a - (b - c)`), so `Eval(node) == EvalTokens(Render(node))` for
every tree (pinned by `TestRenderFaithful`). `Parse` (`mathcore/parse.go`) is the
structural inverse of
`Render`, mirroring the evaluator's grammar: for any canonical expression
`Render(Parse(s)) == s` and the parsed tree evaluates to `EvalTokens(s)` (the
tree is recovered up to the associativity of a same-precedence run). `\text{}`
(WORD) has no AST and is rejected. Pinned by `TestParseRoundTrip*` and
`TestRenderFlowsThroughPipeline`. `EvalTokens` evaluates by parsing the stream
into this AST (`Parse`) and folding it (`Eval`), so the grammar has a single
implementation. (`ComputeProblemDifficulty` and `DetectProblemTypeBitmap` still
read the token stream directly.)

Storage keeps the **original notation** (`\frac{1}{2}`, `\times` render
through KaTeX); normalization is a parsing concern. Only the stage-1.5 `?`
splice mutates stored text (`Admission.Expr`; `spliceLoneLetterRaw` recovers
the splice point in un-normalized text, falling back to the normalized form
on dialect ambiguity).

**Stamp-time structural invariant.** `NormalizeProblemBitmap`
(`mathcore/stamping.go`) OR's in implied bits at every final stamp site: ≥2 distinct core ops or PEMDAS
⇒ CHAINED_OPERATIONS; MISMATCHED ⇒ FRACTIONS. It only ever NARROWS the
serving audience. It exists because the WORD validator reports topic features
as independent items and can omit an implied one; the parser path co-sets them
from the token stream and never needs it.

Every drop is counted in a per-call funnel line (#230):
`funnel: requested= returned= lexer= unknown_rules= collision= answer= envelope= validator= create= inserted=`
(`generationFunnel.String`, `api/generation_funnel.go`; the `lexer`/`unknown_rules` stages
are the ones `mathcore.AdmitExpression` produces).

## Local-first validation

The deterministic tool is authoritative wherever it can operate:

| Problem class | Answer check | Envelope | Topic bits | LLM calls |
|---|---|---|---|---|
| Symbolic (incl. `?`/variable equations, fractions, decimals, `%`) | exact `big.Rat` evaluator | bit-subset check | parser | **zero** |
| WORD (prose) | LLM validator + in-code form eval | LLM validator (same constraints as the generator) | validator's features line + form bits | one |

A WORD problem also carries a `symbolic_expression` (the bare computation it
asks for; see the Difficulty formula section). It is checked in-code to lex and
evaluate to the answer, its detected bits are folded into the stamp, and the
validator's line 4 confirms it uses the operations the problem actually requires
(`FORM_MISMATCH` on a NO) — catching a form that hits the answer with the wrong
computation, which the exact evaluator alone cannot. Difficulty is scored from
the form.

Disagreement = reject, not auto-correct. The answer check, the PEMDAS
dual-eval, and bit detection share one evaluator over one token stream
(`evaluator.go`, `EvalTokens`), so difficulty, bits, and answers cannot
disagree about what an expression means.

**PEMDAS dual-evaluation rule** (`requiresPEMDAS`, `evaluator.go`): evaluate
each equation side twice — correct (recursive descent, precedence + parens,
`EvalTokens`) vs naive (parens stripped, strict left-to-right fold,
`EvalTokensNaiveLTR`). PEMDAS fires iff they disagree. `(3 + 5) * 2` does NOT
fire (the parens spell out the natural order); `12 - (5 - 3)` fires with no
multiplication at all. A naive division-by-zero where correct succeeds counts
as disagreement; a correct-side error is malformed and never fires. Unknowns
are bound to fixed rational probes (`pemdasProbes`) — the formula stays a pure
function of the expression because the recompute fast-path depends on that.

## Difficulty formula (v0.3) and ceiling

`ComputeProblemDifficulty(expression, symbolic_expression)`
(server/mathcore/difficulty.go); the version string is `DifficultyVersion`
(pinned by the anchor and tracked in
[generator-versions.md](generator-versions.md)):

```
magnitude = log10(maxMagnitude + 1) + 0.3      (digit-based for decimals)
opWeight  = max over present ops (table above)
concept   = product of enabled concept multipliers (table above)
structure = 1 + 0.15*(numOps - 1), +0.2 if missing-number
raw       = magnitude * opWeight * concept * structure
scaled    = 1 + 19 * (ln(raw+1) - ln(1.5)) / (ln(16) - ln(1.5))
```

Open-ended scale: floored at 1.0, **no upper clamp** (`compressRaw`; inputs
are bounded by construction; system max ≈ 62). 1–20 is the band for one/two-
concept problems; scores above 20 mean multi-concept stacks. Illustrative
anchors: `3 + 5` ≈ 3.6 · `47 + 28` ≈ 6.5 · `9 × 12` ≈ 9.1 ·
`3x + 7 = 22` ≈ 15.7. **The canonical numbers live in
`TestComputeProblemDifficulty_ReferenceValues`** (difficulty_test.go) —
that test owns them; this table is prose.

`ComputeProblemDifficulty` delegates to `ComputeDifficultyBreakdownFor`, the
single source of truth for the expr-vs-symbolic dispatch;
`DifficultyBreakdown` exposes the intermediate factors for the admin
calibration page without affecting scoring.

Changing the formula in ANY way requires bumping `DifficultyVersion` and
running `recompute_problem_difficulty` on deploy. Calibration: #35.

**Word problems (v0.3):** a word problem's `expression` is prose inside
`\text{...}`, so its operators are invisible to the token-level
`opWeight`/`structure` (the prose rule). It instead carries a
`symbolic_expression` — the bare computation it asks for (e.g. `9999 / 3 / 3`)
— and difficulty is scored from THAT, with the word concept forced on
(`ComputeDifficultyBreakdownFor`'s `symbolic != ""` branch passes `forceWord`
to `computeBreakdown`). So a division word problem scores like its symbolic
twin plus the word bonus, not as addition. `symbolic_expression` is empty for
non-word problems (whose `expression` is already symbolic) and is never shown
to the student: the generator emits it and the WORD validator checks it lexes
and evaluates to the answer before storing it. A legacy word problem with no
`symbolic_expression` falls back to scoring its prose. Issue #266 (`llm_0.5`
in [generator-versions.md](generator-versions.md)).

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

Shared shape constants (generator mapping AND ceiling, lockstep, pinned by the
anchors): `MaxChainLen`, `LargeMaxOperand` (difficulty.go). `MinTargetDifficulty`
floors the selectable target so it never aims below the band the default
envelope populates (mirrored as `MIN_TARGET_DIFFICULTY` in
`web/src/bitmap_validation.js`).

## Selection

- Bitwise-subset SQL in `getSatisfyingProblemIds`,
  `getSatisfyingProblemIdsForTopic` (+ `(bitmap & topic) != 0`), and
  `getDueReviewProblem`. Zero-bitmap rows are excluded defensively (a zero
  bitmap is a subset of everything).
- Index: `(disabled, difficulty, problem_type_bitmap)` — the trailing bitmap
  column makes the subset filter covering (plans in the comment block of
  `migrations/39.sql`).
- **`WEIGHTED_TOPIC_MASK`** (`problem_type.go`) = all bits except MEDIUM/LARGE_NUMBERS.
  Gates `chooseWeightedTopic`, `recordTopicAttempt`, `initTopicStats`: a bit is
  a practice topic iff per-topic difficulty coheres for it; magnitude IS
  difficulty, so "weak at LARGE_NUMBERS → serve large numbers, easier"
  fights itself. Size progression is target_difficulty's job.
- **Pool-supply weighting** (server/api/pool_supply.go): thin-pool topics
  get extra lottery weight (cached per-bit counts), so hard-to-generate bits
  stay in rotation by weight, not by force. Both lottery signals — per-kid
  skill (demand, topic_stats.go) and pool supply — act at serving time; a
  picked-but-thin topic also triggers background generation.

(Selection internals are owned by [selection.md](selection.md); the rows above
are the generation-relevant surface.)

## Generation

- **Prompt** (`mathcore.BuildBitConstraints`, `server/mathcore/prompt_guidance.go`):
  per-bit MAY/MUST NOT pairs, a 3-state magnitude clause, a 2-state chain
  clause, the unknown rules whenever MISSING/SINGLE_VARIABLE is enabled, and
  the closed-world clause ("use ONLY what is explicitly allowed — no square
  roots, exponents, ..."). All constraints are simultaneous. Every constraint
  the insert pipeline enforces must also be communicated here, or the
  generator wastes output on shapes that always reject.
- **Heuristic generator** (server/generator, `heuristic_2.0`): compositional and
  difficulty-targeting (`BuildProblem(bitmap, target, rng)`). It grows the AST
  outward from a chosen answer via one recursion — operators are node choices,
  concepts are operand realizations in the split — so concepts COMPOSE and it
  covers every non-WORD bit and arbitrary stacks, including DECIMALS / PEMDAS /
  PERCENTAGES / SINGLE_VARIABLE (previously LLM-only, #227). A knob inverter
  (`RawForDifficulty` → a magnitude/chain/concept budget, minimal-concept-first,
  binding the shared difficulty constants) sizes each attempt; generate-and-select
  over the canonical pipeline keeps the closest in-window survivor and fails
  closed (closest-achievable, then a deterministic fallback, near the ceiling).
  Version history and the construction design live in
  [generator-versions.md](generator-versions.md).
- **LLM generator** (server/llm_generator, `llm_0.5`): one batched OpenAI call
  (`MAX_QUANTITY = 20`); the `BuildBitConstraints` block is the sole shape
  guidance (`Options.Constraints` is opaque to the package). Emits
  `symbolic_expression` for word problems (`generate_problem.go` prompt). Model
  defaults are owned by [generator-versions.md](generator-versions.md).
- **WORD validator** (`llm_generator.ValidateWordProblem`): one LLM
  round-trip, 3 lines (4 when a `symbolic_expression` was sent) —
  answer (authoritative for prose math) / envelope YES-NO (judged against
  the same constraints the generator saw; `ErrEnvelopeMismatch`) / observed
  features (closed name list, stamps the WORD problem's topic bits —
  generator self-report is never trusted, #224) / form-match YES-NO,
  appended only when a `symbolic_expression` is present (`PROMPT_VALIDATION_FORM`):
  does the form use the operations and numbers the problem actually requires?
  (`ErrFormMismatch` on a NO, #266).

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

When a change requires these steps, record them at the bottom of the commit
message so they reach the PR and the deploy window — see `docs/ops-runbook.md`
→ "When a generation/difficulty change is part of the deploy".

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

1. Constant in `server/mathcore/problem_type.go` + `problemTypeNames` entry — feature-named, not subject-named.
2. Frontend constant in `web/src/enums.js`.
3. Lexer token(s) for new notation (`server/mathcore/expression.go` — the alphabet is the single source of truth).
4. Normalizer synonyms for LaTeX/unicode dialect forms (`normalizeReplacer`).
5. `parseProblemFeatures` field + detection logic (token-level; the prose rule applies).
6. `DetectProblemTypeBitmap` mapping line.
7. Validation-tier decision: extend the evaluator? per-bit deterministic verifier? or WORD-class LLM validation?
8. Difficulty factor + reference values in `TestComputeProblemDifficulty_ReferenceValues` + **DifficultyVersion bump** + recompute on deploy.
9. Reachability rules: per-problem exclusivity with existing bits? → prompt clause + insert reject + ceiling either/or entry (all three sites, always together).
10. `BuildBitConstraints` MAY/MUST-NOT pair.
11. `WEIGHTED_TOPIC_MASK` membership (test: does per-topic difficulty cohere?).
12. Settings dependency rules + `web/src/bitmap_validation.js` error code if needed.
13. UI: group placement (verb / noun-kind / noun-size / framing), label, helper text — against `/style-guide`.
14. Heuristic generator support: a split/leaf realization for the bit in `heuristic_2.0`'s `expand` recursion (`server/generator/heuristic2.go`), or an explicit LLM-only deferral.
15. Backfill: do legacy rows need re-stamping? (re-run `recompute_problem_type_bitmap`.)
16. Update THIS DOCUMENT — the doc-sync test fails CI if you skip the anchors.

## Related files

- `server/mathcore/problem_type.go` — `ProblemType` bits, `problemTypeNames`, `ALL_PROBLEM_TYPES`, `WEIGHTED_TOPIC_MASK`
- `server/mathcore/expression.go` — `NormalizeExpression`, `LexExpression`, `RewriteLoneVariable`, `CountDistinctUnknowns`, `lexNumber`
- `server/mathcore/evaluator.go` — `EvalTokens`, `EvalTokensNaiveLTR`, `requiresPEMDAS`, `pemdasProbes`
- `server/mathcore/stamping.go` — `AdmitExpression`, `DetectProblemTypeBitmap`, `NormalizeProblemBitmap`, `VerifyAnswerSymbolic`, `EnvelopeViolation`
- `server/mathcore/difficulty.go` — `ComputeProblemDifficulty`, `ComputeDifficultyBreakdownFor`, `computeBreakdown`, `compressRaw`, `MaxDiffForBitmap`, the `Concept*`/`Weight*`/`Structure*` constants, `DifficultyVersion`, `MaxChainLen`, `LargeMaxOperand`, `SmallMaxOperand`, `MediumMaxOperand`
- `server/mathcore/prompt_guidance.go` — `BuildBitConstraints`, `ValidatorFeatureNames`
- `server/mathcore/answer_compare.go` — `AnswersEquivalent`
- `server/api/generation_funnel.go` — `generationFunnel`, `VerifyAnswer`, `RewriteLetterInProse` (api-side admission bookkeeping)
- `server/generator` — `heuristic_2.0`: `BuildProblem`, the knob inverter, the compositional `expand` recursion
- `server/llm_generator` — `GenerateProblem`, `ValidateWordProblem`, `PROMPT_QUESTION`, `PROMPT_VALIDATION_WORD`, `PROMPT_VALIDATION_FORM`
