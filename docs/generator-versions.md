# Problem generator versions

The provenance ledger for the two generators. Every problem stores a `generator` string recording
which version of which generator produced it; this doc is the human-readable history behind those
strings. The `generator` column is written once at create time and **never modified**, so
old-version problems stay in the pool and are served normally after new versions ship.

**Update contract.** Bump the `VERSION` constant in the generator package AND add an entry here in
the same PR. `server/api/docs_sync_test.go` (`TestDocsSyncGeneratorVersions`) fails CI when either
anchor below disagrees with the live `generator.VERSION` / `llm_generator.VERSION`, and
`make docs-check` fires when the owned files change without this doc being touched — so a version
bump cannot land undocumented.

This doc owns **generator version provenance only**. The difficulty formula, the open-ended scale,
the `DifficultyVersion` history, and selection/serving mechanics are owned by
[problem-generation.md](problem-generation.md) and [selection.md](selection.md) — point there,
don't re-derive them here.

<!-- BEGIN DOC-SYNC ANCHORS (parsed by server/api/docs_sync_test.go) -->
```
heuristic_version: heuristic_2.0
llm_version: llm_0.5
```
<!-- END DOC-SYNC ANCHORS -->

## The model

Two generators, each with its own `VERSION` string and its own version line. A problem's stored
`difficulty` is the score the admission pipeline computes from the expression
(`api.ComputeProblemDifficulty`), never the requester's target — so a `heuristic_0.0` and an
`llm_0.5` problem at the same stored difficulty are genuinely comparable. A formula-version bump
re-scores legacy problems via `recompute_problem_difficulty`; the version strings themselves are
permanent (see [problem-generation.md](problem-generation.md)).

| Generator | Package | Current version | Nature |
|-----------|---------|-----------------|--------|
| Heuristic | `server/generator` | `heuristic_2.0` (`VERSION`) | Deterministic in-process Go; no API, no cost, fast. Difficulty-targeting, compositional. Default for non-word generation. |
| LLM | `server/llm_generator` | `llm_0.5` (`VERSION`) | Calls OpenAI; richer/varied, especially word problems. Slower, costs per problem, batched up to `MAX_QUANTITY` per call. |

## Heuristic versions

| Version | What it is |
|---------|-----------|
| `heuristic_0.0` | Original hand-written generator. Add/sub/mul only, wired up only for add/sub at low difficulty; output wrapped single numbers in parens (`(3)+(5)-(2)`); no grade awareness; no fractions. Problems remain in the DB for history. |
| `heuristic_1.0` | Template-enumeration rewrite. Four operations (`+ - * /`); fixed template shapes (basic binary, missing-number, multi-term chains, same/different-denominator fractions). Did NOT target difficulty — it emitted a shape and let the formula score whatever fell out, and DECIMALS/PEMDAS/PERCENTAGES/SINGLE_VARIABLE were LLM-only (#227). Problems remain in the DB and serve normally. |
| `heuristic_2.0` (current) | Compositional, difficulty-targeting rewrite (#283). Takes the envelope bitmap + the user's `target_difficulty` and aims each candidate at it; covers EVERY non-WORD bit and arbitrary STACKS of them (the previously-LLM-only DECIMALS/PEMDAS/PERCENTAGES/SINGLE_VARIABLE included). Builds answer-first on the `mathcore` render-only AST; see below. |

### `heuristic_2.0` — compositional answer-first difficulty targeting

`heuristic_2.0` (`BuildProblem(bitmap, target, rng)`) grows the AST OUTWARD from a chosen answer
via one recursion (`expand`): every node's value is known as it is built, operators are node
choices, and concepts are operand realizations chosen in the split (a fraction/decimal/percent/
negative operand) or at a leaf. Concepts therefore COMPOSE — a single problem can carry a fraction,
a division, and a decimal at once — with no per-concept template dispatch. A value concept can be a
direct operand of ANY operator, including `*` and `/` (`3/8 * 5/3`, `0.2 * 3`, `4/5 / 2`): the
multiplicative splits factor the fraction/decimal operand out of the answer (the slash convention
that keeps these unambiguous is in [problem-generation.md](problem-generation.md)). Per-node
invariants keep candidates clean by construction (the integer-division split keeps the dividend an
exact multiple `a = v*b`; integers stay integers unless a value concept is active; values stay
non-negative unless NEGATIVES is on). A knob inverter (`mathcore.RawForDifficulty(target)` → a magnitude/chain/concept budget,
minimal-concept-first, binding to the shared `mathcore` difficulty constants) sizes each attempt,
and **generate-and-select** over the canonical pipeline (`AdmitExpression` + `VerifyAnswerSymbolic`
+ `DetectProblemTypeBitmap` + `EnvelopeViolation` + `ComputeProblemDifficulty`) keeps the closest
in-window survivor. It **fails closed** — a construction slip costs a retry, never a wrong or
out-of-envelope problem — and a near-ceiling or coarse-concept cell degrades to the closest valid
problem, then a deterministic fallback, never a spin. PEMDAS is emergent (a multiplicative subtree
read after an additive operator) and gated by the canonical `requiresPEMDAS` via the envelope check.
**No `DifficultyVersion` bump** — the formula is unchanged; this is a generation-side change only.

## LLM versions

| Version | What changed |
|---------|--------------|
| `llm_0.1` | First prompt: one generic prompt, difficulty given to the LLM as "age in years" with a self-assessed difficulty returned, answer-correctness validation. No grade context. |
| `llm_0.2` | Curriculum alignment (WS2): grade-level Common Core context + few-shot examples from `curriculum.json`; validation also checks grade appropriateness; topic variety hints; rejects problems whose self-reported difficulty diverges >100% from target. |
| `llm_0.3` | Bitmap constraint block: the api-built MAY / MUST NOT block (`api.BuildBitConstraints`) becomes the sole shape guidance; curriculum context, few-shot examples, and "age in years" framing removed (`curriculum.json` deleted). Self-report no longer trusted — features stamped by the detector, difficulty by `ComputeProblemDifficulty`. Validation local-first: symbolic problems answer-checked in-code, word problems get one validator round-trip. Storage preserves original notation. |
| `llm_0.4` | Prompt-only (#249): tells the model to write the ENTIRE word problem as prose and never append the arithmetic or its result, using symbolic math outside `\text{}` only when the statement itself is an expression to manipulate. Fixed ~84% of `llm_0.3` word problems leaking the computation. No code path changed. |
| `llm_0.5` (current) | `symbolic_expression` for word problems (#266) — see below. |

### `llm_0.5` — symbolic_expression for word problems

Word problems were being scored as addition: the difficulty formula reads operators from tokens,
but a story problem's operation lives in prose inside `\text{}`, invisible to it. The generator now
emits a `symbolic_expression` alongside each word problem (`PROMPT_QUESTION`) — the exact
computation the problem asks for, in the same operations and numbers the student would use, not
merely something that hits the answer. It is **stored, never shown**, and validated three ways: it
must lex and evaluate to the answer in-code, and the WORD validator's form line
(`PROMPT_VALIDATION_FORM`) confirms it matches the problem's actual operations, returning
`ErrFormMismatch` on a NO. Difficulty is scored from the form with the word concept applied, so a
division word problem scores like its symbolic twin plus the word bonus. Non-word problems leave it
empty (their `expression` is already symbolic). This version carries a `DifficultyVersion` bump and
a `recompute_problem_difficulty` run on deploy (see [problem-generation.md](problem-generation.md)).

## Invariants

- **The `generator` string is write-once.** Never rewrite it on existing problems; old versions
  coexist in the pool and serve normally.
- **Every candidate stays inside the envelope.** `heuristic_2.0` runs every rendered candidate
  through the canonical admission pipeline and an explicit `EnvelopeViolation` check before
  accepting it, so a candidate whose detected bits (magnitude included) fall outside the requested
  bitmap is rejected, not served. When no candidate lands in the target window after a bounded
  number of attempts it returns the closest valid one; when none is valid it falls back to a small
  envelope-safe problem (`BuildProblem`).
- **Self-report is not trusted (`llm_0.3`+).** Stored features come from the admission detector and
  stored difficulty from `ComputeProblemDifficulty`, not from the generator's own output.
- **Stored difficulty is the computed score, not the requested target** — the source of
  cross-version comparability.

## Gotchas

- **Models are not part of the version string.** LLM generation defaults to `openai.GPT5Nano`
  (`GenerateProblem`), overridable per request via `Options.Model` (used by
  `cmd/diagnose_generation` for model-tier A/B). The WORD validator uses `openai.GPT5`
  (`ValidateWordProblem`), with a cheaper-model override (`ValidateWordProblemWithModel`). Swapping
  the model does **not** bump `VERSION`, so the same `llm_0.5` string can cover problems generated
  by different models.
- **The LLM tags missed word problems after the fact** — `GenerateProblem` adds the `word` feature
  to any returned problem whose expression contains letters even if the model omitted it.

## Adding a new version

Bump when you ship material changes to a generator:

- Small prompt tweak or parameter adjustment → patch (`0.1` → `0.2`)
- New templates, operations, or configs → minor (`1.0` → `1.1`)
- Complete rewrite or incompatible output format → major (`1.0` → `2.0`)

Then, in the SAME PR: (1) update the `VERSION` constant in the generator package; (2) add an entry
above; (3) update the matching anchor (`heuristic_version` / `llm_version`) or
`TestDocsSyncGeneratorVersions` fails CI. Old-version problems remain in the DB and serve normally.

## Related files

- `server/generator/heuristic2.go` — heuristic `VERSION`, `BuildProblem`, the knob inverter
  (`planConfig`/`chooseConcepts`), the compositional `expand` recursion, and the value splits.
- `server/llm_generator/generate_problem.go` — LLM `VERSION`, `PROMPT_QUESTION`, `MAX_QUANTITY`,
  default model.
- `server/llm_generator/validate_problem.go` — `ValidateWordProblem`, `PROMPT_VALIDATION_FORM`,
  `ErrFormMismatch`, validation model.
- `server/api/docs_sync_test.go` — `TestDocsSyncGeneratorVersions` (anchor gate).
- [problem-generation.md](problem-generation.md) — difficulty formula, scale, `DifficultyVersion`,
  admission pipeline, generation routing.
