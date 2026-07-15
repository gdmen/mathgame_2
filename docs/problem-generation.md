# The problem-generation system

The canonical reference for how problems are generated, stamped, validated,
selected, and scored. **If you change any behavior described here, update
this document in the same PR.** Mechanical enforcement: a slim doc-sync test
(`server/api/docs_sync_test.go` `TestDocsSync`) fails CI when the anchors below
disagree with the code, so a new bit or a formula-version bump cannot land
undocumented. Generator version strings (`heuristic_*`, `llm_*`) and the
`DifficultyVersion` history live in the sibling doc
[generator-versions.md](generator-versions.md) — point there, don't inline
them.

<!-- BEGIN DOC-SYNC ANCHORS (parsed by server/api/docs_sync_test.go) -->
```
difficulty_version: 0.5
max_chain_len: 5
max_word_chain_len: 3
min_constructible_operand: 2
large_max_operand: 9999
valid_bitmap_count: 8784
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
| `NEGATIVES` | unary-minus number token; the final stamp also ORs it in when the ANSWER is negative (`3 - 8` = -5 — see the stamp-time invariant) | `ConceptNegatives` |
| `WORD` | `\text{...}` present | `ConceptWord` (stacks with SINGLE_VARIABLE) |
| `MEDIUM_NUMBERS` | maxMagnitude 13–99 (bracket) | via magnitude |
| `LARGE_NUMBERS` | maxMagnitude ≥ 100 (bracket — `1 + 999` is LARGE, not MEDIUM) | via magnitude |
| `CHAINED_OPERATIONS` | numOps ≥ 2 (`=` does not count); on WORD problems, validator-observed (multi-step prose), and OR'd in by the stamp-time invariant whenever ≥2 core-op bits or PEMDAS are set | structure `+StructurePerExtraOp` per op beyond the first |
| `MISSING_NUMBER` | a single `?` outside `\text{}` | structure `+StructureMissing` |
| `DECIMALS` | symbolic decimal token | `ConceptDecimals` |
| `PEMDAS` | the dual-evaluation rule (below); never on WORD problems (suppressed in scoring AND dropped from the stamp — see "Word problems") | `ConceptPEMDAS` |
| `SINGLE_VARIABLE` | variable letter with a coefficient (`3x`) or multiple occurrences (`x + x`); on WORD problems, validator-observed (pure-prose algebra) | `ConceptVariable` |
| `PERCENTAGES` | symbolic `n%` token (evaluates as n/100); a percent literal appears ONLY as `n% of X` — the `of` connective lexes as multiplication, so the bit always co-fires with MULTIPLICATION | `ConceptPercent` |

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
detection never does — WORD problems' topic bits come from the symbolic
skeleton (`symbolic_expression`) they were narrated from, parsed in-code, not
from the prose.

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

**Settings-level dependency rules** — at least one core operation; LARGE ⇒
MEDIUM; MISMATCHED ⇒ FRACTIONS; PEMDAS ⇒ CHAINED; PERCENTAGES ⇒
MULTIPLICATION + MEDIUM_NUMBERS (a percent problem asks for a percent OF a
quantity, and every useful percent literal is itself a two-digit value, so a
small-bracket envelope can construct no percent problem). This doc is the canonical
statement; the rules are enforced client-side in `web/src/bitmap_validation.js`
(the settings UI — API clients bypassing it degrade gracefully) and mirrored,
WORD excluded, in `server/mathcore/bitmap.go` (`ValidBitmap`) for the servable
non-WORD envelope space. Keep the two in sync with this doc when the rules
change.

**The valid non-WORD bitmap space.** `EnumerateValidBitmaps`
(`server/mathcore/bitmap.go`) materializes every bitmap that passes `ValidBitmap`
— exactly **8,784** (`valid_bitmap_count` anchor): per core-op combo, 3
(MISMATCHED/FRACTIONS) × 3 (PEMDAS/CHAINED) × 2⁴ other free bits = 144 per
bracket-percent state; the three brackets (none / MEDIUM / MEDIUM+LARGE)
carry PERCENTAGES as a free bit only when MULTIPLICATION is present and the
bracket is ≥ MEDIUM (5 states for mul combos, 3 otherwise): 8 mul combos ×
720 + 7 non-mul combos × 432. It
walks `0..ALL_PROBLEM_TYPES` filtering by `ValidBitmap` (~65k
pure bit-tests). The admin bitmap × difficulty coverage matrix
(`server/api/admin_bitmap_matrix.go`) live-generates one heuristic_2.0 example
per (bitmap, difficulty) cell across this whole space.

The read side of that report folds pool usage back through a **stamped→settings
normalization**: stamping is bracket-based, so a magnitude ≥ 100 stamps
LARGE_NUMBERS *alone*, while these settings rules require LARGE ⇒ MEDIUM. Before
matching a stamped pool row to an enumerated envelope, OR MEDIUM_NUMBERS into any
stamped bitmap carrying LARGE_NUMBERS (`computeBitmapMatrixReport`). This is the
only stamped/settings divergence for non-WORD rows.

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
[1.6] REDUCE      labeled direct computation collapses to the bare form:
                  an unknown ALONE on one side of '=' with no unknown on the
                  other ("? = 100 - 25", "100 - 25 = ?") is an answer label,
                  not a solve-for-the-blank -> stored as "100 - 25", no
                  MISSING_NUMBER stamp, no missing structure bump. A genuine
                  operand unknown ("? + 10 = 30") is kept. Symbolic-only
                  (prose-carrying expressions pass through).
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

**Division vs. fraction notation.** A bare `/` is always a *fraction* literal
(`3/8`, spacing-agnostic — `6 / 8` is the fraction six-eighths, not division);
division is the **obelus** `÷` (`6 ÷ 2`). NORMALIZE folds `\frac{a}{b}`→`a/b`
and `\div`→`÷` before lex/eval/detection, and the lexer reads `÷` as the
division operator (its AST op stays `/`), so the whole pipeline sees one form.
`mathcore.Render` emits `÷` for division and unspaced `a/b` for fractions, so
any rendered expression — a fraction under division included (`58/3 ÷ 8`,
`3/4 ÷ 2/3`) — is unambiguous without relying on spacing.

**The percent connective.** A percent literal may appear ONLY as `n% of X`
(`25% of 80`): the keyword `of` lexes as the multiplication operator (its own
lexer case, ahead of the letter/variable path), and the placement is enforced
at lex — each percent number must be immediately followed by `of` and each
`of` immediately preceded by a percent number, so `50% + 25%`, `80 × 25%`,
and a bare trailing `25%` are lexer rejects (blocked by default: conversion
equations, complements, and what-percent unknowns are deliberate later
extensions). Past the lexer `of` IS multiplication — detection, the
evaluator, PEMDAS dual-eval, and the difficulty formula see the same tree as
a `*`, so the connective carries no formula change. `Render` emits the
percent-of form for a multiplication whose left factor is a percent literal
(precedence-parenthesizing the right operand: `25% of (40 + 28)`), and the
display skin renders ` of ` as `\text{ of }` (KaTeX-safe), which NORMALIZE
folds back. `25% of ? = 5` (find-the-whole) works — the percent stays left of
`of`. Migration 48 rewrote the pool's legacy `n% × X` rows to the connective
and retired the out-of-grammar percent shapes. `Render` is also
**faithful**: it parenthesizes an operand
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

`heuristic_2.0` splits the two forms across two columns: `expression` holds a
**valid-LaTeX display skin** (`mathcore.DisplayExpression` folds `a/b`→`\frac`,
`÷`→`\div`, `*`→`\times`, ` of `→`\text{ of }`, `%`→`\%` — KaTeX reads a
bare `%` as a comment — and parenthesizes a negative literal that follows an
operator: `9 + (-4)`, the print convention; a LEADING negative and an equation
RHS stay bare, `-15 ÷ 3`, `? - 3 = -7`),
and `symbolic_expression` holds the canonical grammar (`58/3 ÷ 8`). NORMALIZE
bridges them — it folds both to the same form (parens around a bare negative
literal strip back off; they are never PEMDAS-load-bearing, unlike parens
around a subexpression) — so
`NormalizeExpression(expression) == NormalizeExpression(symbolic_expression)`
(pinned by `TestDisplaySymbolicRoundTrip`).

**Stamp-time structural invariant.** `NormalizeProblemBitmap`
(`mathcore/stamping.go`) OR's in implied bits at every final stamp site: ≥2 distinct core ops or PEMDAS
⇒ CHAINED_OPERATIONS; MISMATCHED ⇒ FRACTIONS; and a **negative ANSWER ⇒
NEGATIVES** — producing a signed number is the negatives skill, but token
detection sees only literals, so `3 - 8` (answer -5) would otherwise stamp
SUBTRACTION alone and be served to no-negatives envelopes. The answer is
therefore an input to the stamp (`NormalizeProblemBitmap(bits, answer)`,
`WordFormBitmap(bits, answer)` — the WORD rule reads the SKELETON's answer);
an unparseable answer contributes nothing. It only ever NARROWS the
serving audience. The structural bits are a defensive no-op on parser output —
the symbolic path and the WORD skeleton co-set implied bits from the token
stream already — but the answer rule is load-bearing on every path.

Every drop is counted in a per-call funnel line:
`funnel: requested= returned= lexer= unknown_rules= collision= answer= envelope= validator= create= inserted=`
(`generationFunnel.String`, `api/generation_funnel.go`; the `lexer`/`unknown_rules` stages
are the ones `mathcore.AdmitExpression` produces).

## Local-first validation

The deterministic tool is authoritative wherever it can operate:

| Problem class | Answer check | Envelope | Topic bits | LLM calls |
|---|---|---|---|---|
| Symbolic (incl. `?`/variable equations, fractions, decimals, `%`) | exact `big.Rat` evaluator | bit-subset check | parser | **zero** |
| WORD (prose) | LLM validator (must match skeleton) + in-code skeleton eval | bit-subset check (skeleton bits) | parser (from the `symbolic_expression` skeleton) | one |

A WORD problem also carries a `symbolic_expression` (the skeleton it was
narrated from; see the Difficulty formula section). It is checked in-code to lex
and evaluate to the answer, and it — NOT the prose — is what stamps the WORD
problem's bits (WORD OR'd onto the skeleton's parsed bits). The validator's
line 2 confirms the prose poses that skeleton (`FORM_MISMATCH` on a NO) —
catching prose that hits the answer with the wrong computation, which the exact
evaluator alone cannot. Difficulty is scored from the skeleton.

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

## Difficulty formula (v0.5) and ceiling

`ComputeProblemDifficulty(expression, symbolic_expression)`
(server/mathcore/difficulty.go); the version string is `DifficultyVersion`
(pinned by the anchor and tracked in
[generator-versions.md](generator-versions.md)):

```
magnitude = log10(maxMagnitude + 1) + 0.3      (digit-based for decimals)
opWeight  = max over present ops (table above)
concept   = product of enabled concept multipliers (table above);
            PEMDAS is SUPPRESSED on word problems (v0.4 - a story solver
            takes operation order from the narrative, not from notation)
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
running `recompute_problem_difficulty` on deploy. (v0.5's change is
detection-side: the `of` connective and its `\text{ of }` normalizer fold
make a legacy spliced form like `25%\text{ of }80` fire MULTIPLICATION where
v0.4 read the connective as opaque prose.)

**Word problems (v0.4):** a word problem's `expression` is prose inside
`\text{...}`, so its operators are invisible to the token-level
`opWeight`/`structure` (the prose rule). It instead carries a
`symbolic_expression` — the bare computation it asks for (e.g. `9999 ÷ 3 ÷ 3`)
— and difficulty is scored from THAT. So a division word problem scores like
its symbolic twin plus the word bonus, not as addition. The word bonus is keyed
on word-ness, not on `symbolic_expression` being present:
`ComputeDifficultyBreakdownFor` scores the `symbolic_expression` when it is set
but applies `forceWord` iff the `expression` carries a `\text{}` block. A word
problem's `symbolic_expression` is never shown to the student: for skeleton
narration (`llm_0.6`+) it IS the heuristic skeleton the prose was narrated from
(scored before narration), and the WORD validator confirms the prose poses it.
A legacy word problem with no `symbolic_expression` falls back to scoring its
prose (`llm_0.5` and earlier; see
[generator-versions.md](generator-versions.md)).

Two v0.4 word rules keep scoring, stamping, and generation aligned:

- **PEMDAS never applies to a word problem.** Operator-precedence parsing is
  a written-notation skill; a story solver takes the operation order from the
  narrative. Scoring suppresses `ConceptPEMDAS` when the word concept applies
  (`computeBreakdown`), the stamp drops the PEMDAS bit from the skeleton's
  bits (`WordFormBitmap`, stamping.go — used by the insert path AND the
  restamp tool, so serving-eligibility matches the score), and word skeletons
  are built from a PEMDAS-stripped envelope so no budget is wasted on a
  multiplier the word problem never earns.
- **Word skeleton chains cap at `MaxWordChainLen` (3 operators).** Deeply
  nested chains have no faithful natural-language story — the WORD
  validator's form check rejects the narration attempts (fail closed) — so
  word problems have a NARRATABLE ceiling below the symbolic one. The
  heuristic's word entry point (`BuildWordSkeletonRaw`) enforces the cap and
  `MaxDiffForBitmap`'s word branch prices it (below), keeping the advertised
  ceiling inside the narratable band. Non-word chains keep `MaxChainLen`.

`symbolic_expression` is not word-only. `heuristic_2.0` stores its canonical
grammar form (`58/3 ÷ 8`) there and puts a display skin (`\frac`, `\div`) in
`expression`; it is scored from the grammar with no word bonus (the display
carries no `\text{}`), and that grammar form is the feedstock a WORD generator
wraps into prose. See the storage note under the notation section above.

**The ceiling, `MaxDiffForBitmap`** — the difficulty of the hardest problem
the enabled bits can express. WHY IT EXISTS: adaptive difficulty ratchets
`target_difficulty` upward on success; without the ceiling the target drifts
above anything the envelope can produce — into a band that is empty BY
CONSTRUCTION — and selection's ±1.5 window never matches again (permanent
fallback churn). **Either/or rules** — when two features can't appear in the
same problem, the ceiling computes each branch and takes the higher, never
multiplying exclusives together (that would claim an unreachable ceiling and
recreate the exact drift the ceiling prevents):

- MISSING_NUMBER and SINGLE_VARIABLE are per-problem mutually exclusive (the
  unknown rules).
- Word and non-word bands price differently (v0.4): the word branch earns
  `ConceptWord` but caps chain structure at `MaxWordChainLen` and never earns
  `ConceptPEMDAS`; the non-word branch keeps the full `MaxChainLen` chain and
  PEMDAS with no word concept.
- `ConceptPercent` multiplies in only when MULTIPLICATION and a ≥ MEDIUM
  bracket are also enabled: a percent literal exists only as `n% of X` (a
  multiplication), and every useful percent literal is a two-digit value
  whose rest-operand needs range, so a small-bracket or ×-less PERCENTAGES
  bit can construct no percent problem — an unguarded ceiling would
  over-claim for legacy user bitmaps that predate the dependency rule
  (measured: a `MUL|PCT` small-bracket envelope built 0 percent problems in
  3,000 attempts while an unguarded ceiling advertised 13.6).

**The floor, `MinDiffForBitmap`** — the symmetric twin (v0.4): the difficulty
of the EASIEST problem the enabled bits can construct. A high-weight envelope
(division-only ≈ 7, multiplication-only ≈ 5.7) bottoms out well above the
global `MinTargetDifficulty`; a target below the floor aims the window at a
band that is empty by construction in the other direction (every serve falls
to the synchronous fallback, and pool inserts never land in-window). Every
non-op bit is permissive (MAY, not MUST), so the floor is the minimum enabled
op-weight at the smallest constructible magnitude
(`MinConstructibleOperand = 2`, a calibrated observation of the generators'
easiest output — `2 ÷ 1`, `2 × 2` — not a formula constant).

**`TargetDifficultyRange` / `ClampTargetDifficulty`** are the single source
of truth for the band `target_difficulty` may occupy:
`[max(MinTargetDifficulty, MinDiffForBitmap), MaxDiffForBitmap]`. Wired at:
the `process_events.go` adjuster (step bounds AND the defensive out-of-band
repair clamp at entry), the SET_TARGET_DIFFICULTY validation, the
settings-PUT clamp, and the UI slider range (`web/src/bitmap_validation.js`
mirrors the band for display only — the server is authoritative; the
Go↔JS lockstep is pinned by the generated
`web/src/difficulty_band_fixtures.json`, see
[settings.md](settings.md)).

Shared shape constants (generator mapping AND ceiling, lockstep, pinned by the
anchors): `MaxChainLen`, `MaxWordChainLen`, `MinConstructibleOperand`,
`LargeMaxOperand` (difficulty.go). `MinTargetDifficulty`
floors the selectable target so it never aims below the band the default
envelope populates (mirrored as `MIN_TARGET_DIFFICULTY` in
`web/src/bitmap_validation.js`).

## Selection

- Bitwise-subset SQL in `getSatisfyingProblemIds` and `getDueReviewProblem`.
  Zero-bitmap rows are excluded defensively (a zero bitmap is a subset of
  everything).
- Index: `(status, difficulty, problem_type_bitmap)` — the trailing bitmap
  column makes the subset filter covering (plans in the comment block of
  `migrations/39.sql`; the `disabled`→`status` column swap is `migrations/46.sql`).

(Selection internals are owned by [selection.md](selection.md); the rows above
are the generation-relevant surface.)

## Generation

- **Heuristic generator** (server/generator, `heuristic_2.0`): compositional and
  difficulty-targeting (`BuildProblem(bitmap, target, rng)`). It grows the AST
  outward from a chosen answer via one recursion — operators are node choices,
  concepts are operand realizations in the split — so concepts COMPOSE and it
  covers every non-WORD bit and arbitrary stacks, including DECIMALS / PEMDAS /
  PERCENTAGES / SINGLE_VARIABLE (previously LLM-only) and NEGATIVES
  (mixed-sign sums, negative dividends/factors/minuends, negative answers — a
  negative ANSWER always rides a negative literal in the expression, since the
  stamp reads expression tokens only). A knob inverter (`RawForDifficulty` → a
  magnitude/chain/concept budget, binding the shared difficulty constants) sizes
  each attempt; concept selection alternates a minimal-concept budget mode with
  a coverage-sampled mode, so enabled MAY bits appear across the whole band
  rather than only where the budget arithmetic demands them; generate-and-select
  over the canonical pipeline keeps the closest in-window survivor and fails
  closed (closest-achievable, then a deterministic fallback, near the ceiling).
  Fraction operands stay proper and small; in a magnitude bracket the
  MEDIUM/LARGE size rides on an integer operand (`24 ÷ 2/3 = 36`), so a
  fraction problem reaches high difficulty without an inflated numerator like
  `58/3`. Pure-additive and mismatched two-fraction cells have no integer to
  carry it, so their hardest buckets stay just below the formula ceiling.
  Version history and the construction design live in
  [generator-versions.md](generator-versions.md). It is the sole source for
  non-WORD problems AND the **skeleton source** for WORD problems (below).
- **WORD generation** (`server/api/generate_problems.go` `runWordGenerator`,
  generator `llm_0.7`): the heuristic builds a scored symbolic **skeleton**
  (`BuildWordSkeletonRaw`) aimed
  at `RawForDifficulty(target) / ConceptWord` — the lower budget that
  lands the narrated word problem near `target` once the word concept multiplies
  it back up — under the word narratability policy (chains capped at
  `MaxWordChainLen`, PEMDAS-stripped envelope; see "Word problems" above) —
  then the LLM (`llm_generator.NarrateProblems`, one batched call,
  `MAX_QUANTITY = 20`) is asked ONLY to dress each skeleton in prose that poses
  that exact computation: no new numbers, no changed operation. Stored:
  `expression` = the prose, `symbolic_expression` = the skeleton, `answer` = the
  skeleton's answer, bits = `WordFormBitmap(skeleton bits)`; difficulty is
  skeleton × word by construction. The heuristic
  owns the math; the LLM never authors it.
- **WORD validator** (`llm_generator.ValidateWordProblem`): one LLM round-trip
  on the stronger model (GPT5, the independent check on the cheaper narrator),
  2 lines — the answer (must equal the skeleton's) and a form YES-NO: does the
  prose pose the skeleton's computation, its operations and numbers?
  (`ErrFormMismatch` on a NO). It fails closed. The skeleton owns the math AND
  the bits, so the validator no longer judges envelope compliance or reports
  features — those come from parsing the `symbolic_expression` in-code.

## Backfill tools and deployment

Single server, single DB. Deploy order matters:

1. Stop the server; deploy the new binary (not serving).
2. Migrations run on startup — but run the backfills BEFORE starting:
3. `recompute_problem_type_bitmap` — restamps every row via the admission
   pipeline. SET semantics (re-runnable). WORD rows with a
   `symbolic_expression` restamp from the SKELETON via `WordFormBitmap`
   (identical to the insert path), and the admitted skeleton is written back
   to `symbolic_expression` so the difficulty tool scores the same text the
   bits were stamped from (a labeled-unknown skeleton collapses in both
   places); skeleton-less legacy WORD rows keep their legacy topic bits OR'd
   in. Lone-letter rows get the `?` splice in expression + explanation
   (+ answer), listed for spot-checking. Reports: lexer token census
   (out-of-alphabet rows), zero-bitmap review list, unknown-rule review list,
   skeleton-reject review list. Run with `-dry-run` first and read the census.
4. `recompute_problem_difficulty` — restamps difficulty (the version bump
   forces every row). MUST run after the bitmap tool (the rewrite mutates
   expressions).
5. Start the server.

When a change requires these steps, record them at the bottom of the commit
message so they reach the PR and the deploy window — see `docs/ops-runbook.md`
→ "When a generation/difficulty change is part of the deploy".

## The new-bit checklist

Every future bit (EXPONENTS is the first consumer) walks these touchpoints. The blocked-by-default design (the
lexer allowlist + admission-pipeline rejects) protects the system between additions — a new concept is
forbidden until deliberately added.

1. Constant in `server/mathcore/problem_type.go` + `problemTypeNames` entry — feature-named, not subject-named.
2. Frontend constant in `web/src/enums.js`.
3. Lexer token(s) for new notation (`server/mathcore/expression.go` — the alphabet is the single source of truth).
4. Normalizer synonyms for LaTeX/unicode dialect forms (`normalizeReplacer`).
5. `parseProblemFeatures` field + detection logic (token-level; the prose rule applies).
6. `DetectProblemTypeBitmap` mapping line.
7. Validation-tier decision: extend the evaluator? per-bit deterministic verifier? or WORD-class LLM validation?
8. Difficulty factor + reference values in `TestComputeProblemDifficulty_ReferenceValues` + **DifficultyVersion bump** + recompute on deploy.
9. Reachability rules: per-problem exclusivity with existing bits? → insert reject + ceiling either/or entry (both sites, always together).
10. Settings dependency rules + `web/src/bitmap_validation.js` error code if needed.
11. UI: group placement (verb / noun-kind / noun-size / framing), label, helper text — against `/style-guide`.
12. Heuristic generator support: a split/leaf realization for the bit in `heuristic_2.0`'s `expand` recursion (`server/generator/heuristic2.go`), or an explicit LLM-only deferral.
13. Backfill: do legacy rows need re-stamping? (re-run `recompute_problem_type_bitmap`.)
14. Update THIS DOCUMENT — the doc-sync test fails CI if you skip the anchors.

## Related files

- `server/mathcore/problem_type.go` — `ProblemType` bits, `problemTypeNames`, `ALL_PROBLEM_TYPES`
- `server/mathcore/expression.go` — `NormalizeExpression`, `LexExpression`, `RewriteLoneVariable`, `CountDistinctUnknowns`, `lexNumber`
- `server/mathcore/evaluator.go` — `EvalTokens`, `EvalTokensNaiveLTR`, `requiresPEMDAS`, `pemdasProbes`
- `server/mathcore/stamping.go` — `AdmitExpression`, `reduceLabeledUnknown`, `DetectProblemTypeBitmap`, `NormalizeProblemBitmap`, `WordFormBitmap`, `VerifyAnswerSymbolic`, `EnvelopeViolation`
- `server/mathcore/difficulty.go` — `ComputeProblemDifficulty`, `ComputeDifficultyBreakdownFor`, `computeBreakdown`, `compressRaw`, `MaxDiffForBitmap`, `MinDiffForBitmap`, `TargetDifficultyRange`, `ClampTargetDifficulty`, the `Concept*`/`Weight*`/`Structure*` constants, `DifficultyVersion`, `MaxChainLen`, `MaxWordChainLen`, `MinConstructibleOperand`, `LargeMaxOperand`, `SmallMaxOperand`, `MediumMaxOperand`
- `server/mathcore/answer_compare.go` — `AnswersEquivalent`
- `server/api/generation_funnel.go` — `generationFunnel`, `VerifyAnswer`, `RewriteLetterInProse` (api-side admission bookkeeping)
- `server/generator` — `heuristic_2.0`: `BuildProblem`, the knob inverter, the compositional `expand` recursion
- `server/llm_generator` — `NarrateProblems`, `PROMPT_NARRATE`, `Skeleton`, `TopicPromptHint`, `ValidateWordProblem`, `PROMPT_VALIDATION_WORD`
