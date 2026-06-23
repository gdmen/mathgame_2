# The settings screen

The parent-facing controls that author a user's problem envelope: the problem-type bitmap and the
target-difficulty slider, plus the playlist/video reward controls. This doc owns the **client** of
the problem-generation system — how the bitmap is composed, validated, and clamped in the UI. The
bit *semantics* (what each bit fires on, its difficulty factor) live in
[`docs/problem-generation.md`](problem-generation.md); this doc points there rather than re-deriving
them.

**Update contract.** Change any behavior in `web/src/settings.js` or `web/src/bitmap_validation.js`
and update this doc in the same PR. `make docs-check BASE=origin/master` enforces that this doc is
touched when its owned files change. The anchor block below is asserted against code constants by
`server/api/docs_sync_test.go` (`TestDocsSyncSettings`) — the ceiling constants against
`server/api/difficulty.go`, the error codes against `bitmap_validation.js`. The dependency rules and
the ceiling formula are a **mirror** of the server-authoritative copies — see Invariants.

<!-- BEGIN DOC-SYNC ANCHORS (parsed by server/api/docs_sync_test.go) -->
```
min_target_difficulty: 3
ceiling_max_chain_len: 5
ceiling_large_max_operand: 9999
ceiling_small_max_operand: 12
ceiling_medium_max_operand: 99
validation_error_codes: NO_CORE_OP, LARGE_REQUIRES_MEDIUM, MISMATCHED_REQUIRES_FRACTIONS, PEMDAS_REQUIRES_CHAINED
```
<!-- END DOC-SYNC ANCHORS -->

## The model

The settings screen exposes the two user controls described in problem-generation.md ("The model"):

- **`problem_type_bitmap`** — the envelope. Authored directly by toggling problem-type chips. Bit
  constants come from `web/src/enums.js` `ProblemTypes` (the frontend copy of
  `server/api/enums.go`).
- **`target_difficulty`** — the adaptive lever, surfaced as a slider whose max IS the bitmap's
  difficulty ceiling.

Settings persist via `POST /settings/{user_id}` (`postSettings`). The bitmap is POSTed only on a
valid commit (`commit` — the `v.valid` branch); the difficulty / work-percentage sliders POST on
mouseup/blur. The whole screen is PIN-gated (`SettingsView` via `RequirePin`).

## Problem-type taxonomy

`PROBLEM_TYPE_GROUPS` places every bit into one of four cards, each answering a single parent
question. Labels are parent vocabulary; the underlying constants are feature-named (see
problem-generation.md "Bit reference").

| Card (`title`) | Question | Bits (in render order) |
|---|---|---|
| Operations | What can your child do? | ADDITION, SUBTRACTION, MULTIPLICATION, DIVISION |
| Number types | What kinds of numbers? | DECIMALS, PERCENTAGES, NEGATIVES, FRACTIONS → MISMATCHED_DENOMINATORS |
| Number size | How big can the numbers be? | MEDIUM_NUMBERS, LARGE_NUMBERS |
| Problem format | How can problems be posed? | WORD, MISSING_NUMBER, SINGLE_VARIABLE, CHAINED_OPERATIONS → PEMDAS |

`→` marks a **dependent** (`dependsOn`) rendered on its own row directly below its **parent**
(`hasDependent`). Two dependent pairs exist: FRACTIONS→MISMATCHED_DENOMINATORS and
CHAINED_OPERATIONS→PEMDAS. A parent with dependents sits at the bottom of its card so the dependent
row falls directly beneath it. The Number size card also carries a `hint`.

Card-to-bit placement is hand-maintained and NOT enforced by a test: every `ProblemTypes` bit
happens to be placed, but a new bit added to `enums.js` will silently not appear on the screen unless
added to `PROBLEM_TYPE_GROUPS` too (see the new-bit checklist in problem-generation.md).

## Toggle behavior

Two layers keep the bitmap coherent as chips toggle:

**Render-time gating** (`ProblemTypesSettingsView`). A dependent chip is `disabled` when its parent
bit is off (`parentOff`) and styled `dep parent-off`; parents carry `has-dep`. The checkbox is
checked iff the bit is set in the current bitmap.

**`applyToggleRules`** — the bit-math run on every toggle, mirroring the dependency rules so the
saved bitmap is always valid:

| Action | Side effect | Why |
|---|---|---|
| enable LARGE_NUMBERS | also sets MEDIUM_NUMBERS | no size gap (mirrors LARGE ⇒ MEDIUM) |
| disable MEDIUM_NUMBERS | also clears LARGE_NUMBERS | keeps LARGE ⇒ MEDIUM |
| disable FRACTIONS | also clears MISMATCHED_DENOMINATORS | clears orphaned dependent |
| disable CHAINED_OPERATIONS | also clears PEMDAS | clears orphaned dependent |

These cover the up-front-fixable rules. `NO_CORE_OP` and `MISMATCHED_REQUIRES_FRACTIONS` are not
auto-fixed by toggling (you can't auto-pick an operation for the parent; enabling MISMATCHED without
FRACTIONS is prevented by render-time gating since MISMATCHED depends on FRACTIONS), so they surface
as validation errors instead.

## Validation — `validateBitmap`

`validateBitmap` returns `{ valid: true }` or `{ valid: false, errors: [{ code, message,
offendingBits }] }`. It encodes the four settings-level dependency rules from problem-generation.md
("Settings-level dependency rules"):

| Code | Fires when | Anchored to card |
|---|---|---|
| `NO_CORE_OP` | no ADD/SUB/MUL/DIV bit set | Operations |
| `LARGE_REQUIRES_MEDIUM` | LARGE_NUMBERS set, MEDIUM_NUMBERS clear | Number size |
| `MISMATCHED_REQUIRES_FRACTIONS` | MISMATCHED_DENOMINATORS set, FRACTIONS clear | Number types |
| `PEMDAS_REQUIRES_CHAINED` | PEMDAS set, CHAINED_OPERATIONS clear | Problem format |

Errors render inside the card they concern: `ERROR_GROUPS` maps each `code` to a card `title`, and
`errorsFor` filters the error list per card. A new error code with no `ERROR_GROUPS` entry would be
computed but never displayed. `errCallback` propagates `!valid` so the host screen can block save.

## The target-difficulty slider — `maxDiffForBitmap`

`TargetDifficultySettingsView` sizes the slider's range to the current bitmap's ceiling: `min =
MIN_TARGET_DIFFICULTY`, `max = maxDiffForBitmap(bitmap)`. The slider re-renders as the bitmap changes
(`onBitmapChange` lifts the bitmap to `SettingsView` state). The displayed value is clamped down to
the ceiling (`shown`); the **server clamps authoritatively on save** — this client copy only sizes
the UI.

Parents see an integer **percent (1–100)**, not the raw formula number (`percent`): the position of
`shown` within `[MIN, ceiling]`. The raw difficulty numbers are formula internals.

`maxDiffForBitmap` is a line-by-line mirror of the server's `MaxDiffForBitmap`
(`server/api/difficulty.go`):

```
maxOperand = 12 | 99 (MEDIUM) | 9999 (LARGE)
magnitude  = log10(maxOperand + 1) + 0.3
opWeight   = max over enabled ops (SUB 1.1, MUL 2.2, DIV 2.8; base 1.0)
concept    = product of enabled concept multipliers
             (FRACTIONS 2.0, MISMATCHED 1.5, NEGATIVES 1.3, WORD 1.3,
              PEMDAS 1.5, DECIMALS 2.0, PERCENTAGES 2.0)
structure  = 1.0; if CHAINED: 1.0 + 0.15 * (MaxChainLen - 1)   // = 1.6
best       = magnitude * opWeight * concept * structure
  if SINGLE_VARIABLE: max(best, base * 5.0 * structure)   // either/or
  if MISSING_NUMBER:  max(best, base-without-x5 * (structure + 0.2))
ceiling    = compress(best)   // 1 + 19*(ln(best+1)-ln(1.5))/(ln(16)-ln(1.5)), floored at 1
```

The **either/or rule** (problem-generation.md "The ceiling"): MISSING_NUMBER and SINGLE_VARIABLE are
per-problem mutually exclusive, so the ceiling takes the higher of the two branches rather than
multiplying both in. The multiplier constants themselves are owned by problem-generation.md and the
server's difficulty tests; this doc cites them as the mirror but does not own them.

## Other settings (non-envelope)

These views live in settings.js but are outside the problem-generation envelope; noted here for
completeness:

- **`TargetWorkPercentageSettingsView`** — a 0–100 slider for `target_work_percentage` (share of
  time on math vs. reward video).
- **`PlaylistsSettingsView`** — add/remove YouTube reward playlists (`GET/POST/DELETE /playlists`);
  accepts a URL (`playlist_url`) or a raw playlist ID (`youtube_playlist_id`).
  `RECOMMENDED_PLAYLISTS` is an empty UI-only curation list, hidden unless populated.
- **`VideosSettingsView`** — read-only list of reward videos (union of the playlists); flags an error
  when fewer than three are enabled (`getEnabledVideoCount`).

## Invariants

1. **Server is authoritative.** Both client rules here are mirrors. The bitmap dependency rules also
   live in server validation, and `MaxDiffForBitmap` is the source of truth for the ceiling. API
   clients bypassing the UI degrade gracefully — the server clamps and re-validates on save.
2. **`maxDiffForBitmap` ⇔ `MaxDiffForBitmap` lockstep.** Any change to the server ceiling formula,
   the multiplier set, `MaxChainLen`, or `LargeMaxOperand` must be reflected here in the same PR, or
   the slider max diverges from the server clamp. The shared constants are anchored in
   problem-generation.md.
3. **The saved bitmap is always valid OR not saved.** `commit` POSTs only when `validateBitmap`
   passes.
4. **Dependent bits never outlive their parent.** Render-time gating plus the `applyToggleRules`
   clears guarantee MISMATCHED ⇒ FRACTIONS and PEMDAS ⇒ CHAINED hold in the composed bitmap.

## Gotchas

- **No floor guard on the slider denominator.** `MIN_TARGET_DIFFICULTY` (3) mirrors the server's
  `MinTargetDifficulty`. The percent computation divides by `ceiling - MIN`, which goes non-positive
  if a bitmap's ceiling falls at or below the floor. In practice the cheapest legal envelope (one
  core op) ceils above 3, so this isn't currently reachable, but there's no guard.
- **No client-side test mirror.** No JS test asserts `maxDiffForBitmap` against `MaxDiffForBitmap`;
  the doc-sync test asserts only the named constants, not the formula body. Drift in the formula body
  would not be caught mechanically — review the two functions together.
- **Card and error placement are hand-maintained.** `PROBLEM_TYPE_GROUPS` and `ERROR_GROUPS` are not
  derived from the enum; a new bit or error code can be computed but invisible until added to these
  maps.

## Related files

- `web/src/settings.js` — `PROBLEM_TYPE_GROUPS`, `applyToggleRules`, `ProblemTypesSettingsView`,
  `ERROR_GROUPS`, `TargetDifficultySettingsView`, `SettingsView`, `postSettings`.
- `web/src/bitmap_validation.js` — `validateBitmap`, `maxDiffForBitmap`, `MIN_TARGET_DIFFICULTY`.
- `web/src/enums.js` — `ProblemTypes` bit constants.
- `server/api/difficulty.go` — `MaxDiffForBitmap`, `MinTargetDifficulty`, `MaxChainLen`,
  `LargeMaxOperand`, `smallMaxOperand`, `mediumMaxOperand` (the authoritative copies).
- [`docs/problem-generation.md`](problem-generation.md) — bit semantics, difficulty formula, the
  ceiling rationale, the new-bit checklist.

## Extension checklist (adding a problem-type bit to the screen)

Walk the full new-bit checklist in problem-generation.md; the settings-screen touchpoints are:

1. Add the bit to `web/src/enums.js` `ProblemTypes` (matches `server/api/enums.go`).
2. Place it in `PROBLEM_TYPE_GROUPS` — pick the card (verb / noun-kind / noun-size / framing), label
   (parent vocabulary), and `dependsOn`/`hasDependent` if it's part of a dependency pair.
3. If it adds a dependency rule: add a check + error code to `validateBitmap`, an `ERROR_GROUPS`
   entry mapping the code to its card, and (if up-front fixable) an `applyToggleRules` clause.
4. If it changes the difficulty formula: update `maxDiffForBitmap` in lockstep with the server
   `MaxDiffForBitmap`.
5. Update this doc and its anchor block (the doc-sync test fails CI otherwise).
