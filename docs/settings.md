# The settings screen

The parent-facing controls that author a user's problem envelope: the problem-type bitmap and the
target-difficulty control, plus the playlist/video reward controls. This doc owns the **client** of
the problem-generation system — how the bitmap is composed, validated, and clamped in the UI. The
bit *semantics* (what each bit fires on, its difficulty factor) live in
[`docs/problem-generation.md`](problem-generation.md); this doc points there rather than re-deriving
them.

**Update contract.** Change any behavior in `web/src/settings.js` or `web/src/bitmap_validation.js`
and update this doc in the same PR. `make docs-check BASE=origin/master` enforces that this doc is
touched when its owned files change. The anchor block below is asserted against code constants by
`server/api/docs_sync_test.go` (`TestDocsSyncSettings`) — the ceiling constants against
`server/mathcore/difficulty.go`, the error codes against `bitmap_validation.js`. The dependency rules and
the ceiling formula are a **mirror** of the server-authoritative copies — see Invariants.

<!-- BEGIN DOC-SYNC ANCHORS (parsed by server/api/docs_sync_test.go) -->
```
min_target_difficulty: 3
ceiling_max_chain_len: 5
ceiling_max_word_chain_len: 3
ceiling_large_max_operand: 9999
ceiling_small_max_operand: 12
ceiling_medium_max_operand: 99
floor_min_constructible_operand: 2
min_playable_videos: 1
validation_error_codes: NO_CORE_OP, LARGE_REQUIRES_MEDIUM, MISMATCHED_REQUIRES_FRACTIONS, PEMDAS_REQUIRES_CHAINED, PERCENTAGES_REQUIRE_MULTIPLICATION, PERCENTAGES_REQUIRE_MEDIUM
```
<!-- END DOC-SYNC ANCHORS -->

## The model

The settings screen exposes the two user controls described in problem-generation.md ("The model"):

- **`problem_type_bitmap`** — the envelope. Authored directly by toggling problem-type chips. Bit
  constants come from `web/src/enums.generated.js` `ProblemTypes`, generated from
  `server/mathcore/problem_type.go` by `make build-api`.
- **`target_difficulty`** — the adaptive lever, surfaced as a meter whose max IS the bitmap's
  difficulty ceiling, nudged rather than dragged.

Settings persist via `POST /settings/{user_id}` (`postSettings`). The bitmap is POSTed only on a
valid commit (`commit` — the `v.valid` branch); a difficulty nudge POSTs on click and the
work-percentage slider POSTs on release/blur. Every POST reports its outcome (see Save feedback). The whole screen is PIN-gated (`SettingsView` calls `RequirePin(user.pin)`; see
[accounts.md](accounts.md)).

## Problem-type taxonomy

`PROBLEM_TYPE_GROUPS` places every bit into one of four cards, each answering a single parent
question. Labels are parent vocabulary; the underlying constants are feature-named (see
problem-generation.md "Bit reference").

| Card (`title`) | Question | Bits (in render order) |
|---|---|---|
| Operations | What can your child do? | ADDITION, SUBTRACTION, DIVISION, MULTIPLICATION → PERCENTAGES |
| Number types | What kinds of numbers? | DECIMALS, NEGATIVES, FRACTIONS → MISMATCHED_DENOMINATORS |
| Number size | How big can the numbers be? | MEDIUM_NUMBERS, LARGE_NUMBERS |
| Problem format | How can problems be written? | WORD, MISSING_NUMBER, SINGLE_VARIABLE, CHAINED_OPERATIONS → PEMDAS |

`→` marks a **dependent** (`dependsOn`) rendered on its own row directly below its **parent**.
Three dependent pairs exist: FRACTIONS→MISMATCHED_DENOMINATORS, CHAINED_OPERATIONS→PEMDAS, and
MULTIPLICATION→PERCENTAGES (a percent problem asks for a percent OF a quantity — `n% of X` is a
multiplication, so PERCENTAGES lives under its parent in Operations rather than in Number types).
`dependsOn` is the **only** place a pair is declared: parenthood (`isParent`, the `has-dep` class),
render-time gating, and the toggle math all read it. A parent sits at the bottom of its card so the
dependent row falls directly beneath it. The Number size card also carries a `hint`.

Card-to-bit placement is hand-maintained and NOT enforced by a test: every `ProblemTypes` bit
happens to be placed, but a newly generated bit will silently not appear on the screen unless
added to `PROBLEM_TYPE_GROUPS` too (see the new-bit checklist in problem-generation.md).

## Toggle behavior

Two layers keep the bitmap coherent as chips toggle:

**Render-time gating** (`ProblemTypesSettingsView`). A dependent chip is `disabled` when its parent
bit is off (`parentOff`) and styled `dep parent-off`; parents carry `has-dep`. The checkbox is
checked iff the bit is set in the current bitmap.

**`applyToggleRules`** — the bit-math run on every toggle, mirroring the dependency rules so the
saved bitmap is always valid. It is **derived**, not a clause per pair: `REQUIREMENTS` is one list
of `{ bit, requires }` edges, and a toggle walks it (`closure`) transitively in one direction or the
other:

| Toggle | Effect |
|---|---|
| enable | also sets everything the bit requires, and what those require |
| disable | also clears everything that required the bit, and their dependents in turn |

The edges come from two places, neither of them a hand-kept mirror of the other:

- every `dependsOn` in `PROBLEM_TYPE_GROUPS` (MISMATCHED⇒FRACTIONS, PEMDAS⇒CHAINED,
  PERCENTAGES⇒MULTIPLICATION), and
- `CROSS_CARD_REQUIREMENTS` — the rules whose two bits sit on **different** cards, so the taxonomy
  has nowhere to say them: LARGE⇒MEDIUM (no size gap) and PERCENTAGES⇒MEDIUM (percent literals are
  two-digit values).

So enabling LARGE or PERCENTAGES pulls MEDIUM in; disabling MEDIUM clears both; disabling a parent
clears its dependent. Adding a `dependsOn` is enough to get the clearing — the failure mode this
replaces was a pair declared in the taxonomy but forgotten in the toggle math, which left an orphan
bit that `validateBitmap` rejects and the screen then silently refuses to save.
`settings_toggle_rules.test.js` pins the invariant over the taxonomy rather than over today's bits:
**every single toggle from a valid bitmap leaves a valid bitmap**.

`NO_CORE_OP` is the one rule with nothing to derive from (you can't auto-pick an operation for the
parent), so it surfaces as a validation error instead.

## Validation — `validateBitmap`

`validateBitmap` returns `{ valid: true }` or `{ valid: false, errors: [{ code, message, bits }] }`,
where `bits` names the bits the error is about. It encodes the settings-level dependency rules from
problem-generation.md ("Settings-level dependency rules"):

| Code | Fires when | Anchored to card |
|---|---|---|
| `NO_CORE_OP` | no ADD/SUB/MUL/DIV bit set | Operations |
| `LARGE_REQUIRES_MEDIUM` | LARGE_NUMBERS set, MEDIUM_NUMBERS clear | Number size |
| `MISMATCHED_REQUIRES_FRACTIONS` | MISMATCHED_DENOMINATORS set, FRACTIONS clear | Number types |
| `PEMDAS_REQUIRES_CHAINED` | PEMDAS set, CHAINED_OPERATIONS clear | Problem format |
| `PERCENTAGES_REQUIRE_MULTIPLICATION` | PERCENTAGES set, MULTIPLICATION clear | Operations |
| `PERCENTAGES_REQUIRE_MEDIUM` | PERCENTAGES set, MEDIUM_NUMBERS clear | Operations |

Errors render inside the card they concern, and that placement is **derived**: `errorCardTitle`
finds the card holding the error's `bits` (`NO_CORE_OP` carries the core-op mask, which is why it
lands on Operations), and `errorsFor` filters the error list per card. An error whose bits belong to
no card falls back to the first card rather than going unrendered — a computed-but-invisible error
means the screen refuses to save with nothing on screen to fix. `errCallback` propagates `!valid` so
the host screen can block save.

## The target-difficulty meter — `targetDifficultyRange`

`TargetDifficultySettingsView` sizes the control to the current bitmap's band:
`{lo, hi} = targetDifficultyRange(bitmap)` — `lo = max(MIN_TARGET_DIFFICULTY,
minDiffForBitmap(bitmap))`, `hi = maxDiffForBitmap(bitmap)`. It re-renders as the bitmap
changes (`onBitmapChange` lifts the bitmap to `SettingsView` state). The displayed value is clamped
into the band (`shown`); the **server clamps authoritatively on save** — this client copy only sizes
the UI. A high-weight envelope (division-only, multiplication-only) floors above the global minimum:
nothing easier is constructible there, so the band's low end starts at what the envelope can
actually build.

The value belongs to the adaptive system, not the parent, so it renders as a **read-only meter**
(no thumb) with two **nudge buttons**. A nudge steps `shown` by a tenth of the band and clamps to
`[lo, hi]` — the same positions the old draggable slider could reach, minus the implication that
the parent sets this number outright. Each nudge saves immediately; the buttons disable at the
band's ends and when the band is degenerate (`lo === hi`).

Parents see an integer **percent (1–100)**, not the raw formula number (`percent`): the position of
`shown` within `[lo, hi]`. The raw difficulty numbers are formula internals.

`maxDiffForBitmap` / `minDiffForBitmap` are line-by-line mirrors of the server's
`MaxDiffForBitmap` / `MinDiffForBitmap` (`server/mathcore/difficulty.go`):

```
maxOperand = 12 | 99 (MEDIUM) | 9999 (LARGE)
magnitude  = log10(maxOperand + 1) + 0.3
opWeight   = max over enabled ops (SUB 1.1, MUL 2.2, DIV 2.8; base 1.0)
concept    = product of enabled concept multipliers
             (FRACTIONS 2.0, MISMATCHED 1.5, NEGATIVES 1.3, DECIMALS 2.0,
              PERCENTAGES 2.0 — counted only when MULTIPLICATION and a
              >=MEDIUM bracket are enabled: a percent exists only as
              "n% of X" with two-digit values)
branch(c, s):  best of c*s | SINGLE_VARIABLE: c*5.0*s | MISSING: c*(s+0.2)
non-word:  branch(concept * [PEMDAS 1.5], 1.0 or 1.0+0.15*(MaxChainLen-1))
word:      branch(concept * WORD 1.3,    1.0 or 1.0+0.15*(MaxWordChainLen-1))
ceiling    = compress(max of the branches)
           // compress: 1 + 19*(ln(x+1)-ln(1.5))/(ln(16)-ln(1.5)), floored at 1

floor      = compress((log10(2 + 1) + 0.3) * MIN over enabled op weights)
           // operand 2 = the easiest constructible problem; concepts and
           // structure are all MAY bits, so the floor ignores them
```

The **either/or rules** (problem-generation.md "The ceiling"): MISSING_NUMBER and SINGLE_VARIABLE
are per-problem mutually exclusive, and the word band prices differently from the non-word band
(word concept but capped chain and no PEMDAS), so the ceiling takes the highest reachable branch
rather than multiplying exclusives in. The multiplier constants themselves are owned by
problem-generation.md and the server's difficulty tests; this doc cites them as the mirror but does
not own them.

## Other settings (non-envelope)

These views live in settings.js but are outside the problem-generation envelope; noted here for
completeness:

- **`TargetWorkPercentageSettingsView`** — a 0–100 slider for `target_work_percentage`, labelled
  "Math / video balance" with both ends named so the direction is unambiguous.
- **`PlaylistsSettingsView`** — add/remove YouTube reward playlists (`GET/POST/DELETE /playlists`);
  accepts a URL (`playlist_url`) or a raw playlist ID (`youtube_playlist_id`).
  Under the parent's list, **`RECOMMENDED_PLAYLISTS`** offers a few hand-picked public playlists
  for a parent who arrives with nothing in mind, drawn on the same row grid as the list above
  (thumbnail, title linking out to YouTube, channel, count chip, outline *Add*) so a playlist looks
  the same before and after adding; only the list it sits in changes. It is a UI-only constant in
  `settings.js` holding copied YouTube metadata (id, title, channel, count, thumbnail): nothing is
  fetched to draw it. *Add* posts `youtube_playlist_id` through the same `addPlaylist` path as a
  pasted URL, so a recommendation gets no special server treatment; each row carries its own busy
  state so one *Adding…* does not grey out the input's Add or the other rows. A recommendation the
  parent already has (matched on `you_tube_id`) is not shown, and the section disappears when
  nothing is left to suggest. Entries are links to other people's playlists: one going private or
  being deleted shows up as the add failing in the shared error line, the same failure a bad
  pasted URL gives, so the only maintenance is replacing the entry.
  Each row is a **disclosure**: opening one lazily fetches
  `GET /playlists/{playlist_id}/videos` and lists that playlist's videos, with unavailable ones
  muted and labelled (never red — unavailable is a status, not a validation error). The card header
  carries the total playable count, which is **`playable_total` from the server, never the sum of
  the rows' `playable_count`**: a video in two of the parent's playlists is one reward but two row
  counts, so a summed total overstates what the reward loop has and can clear
  `MIN_PLAYABLE_VIDEOS` when the game cannot. `playable_total` comes from the same
  `countEnabledVideosForUser` helper `/pageload` and `/play` use, so every surface that gates on
  the floor is reading one number — and one floor: `MIN_PLAYABLE_VIDEOS` and the server's
  `minPlayableVideos` are the same value, held together by the `min_playable_videos` anchor
  above, so an account the client calls unplayable cannot get a game by asking `/play` directly
  (see [accounts.md](accounts.md) and [gameplay.md](gameplay.md)).
  When the total is short, the requirement is stated beside the red tally
  (`.playlist-total-need`) — there is no separate error line for it. **Removing a playlist takes
  effect immediately and offers an undo** for `UNDO_WINDOW_MS` (30s) in a `.playlist-undo` list
  item occupying the removed row's slot (so the list does not reflow); nothing is confirmed up
  front. Removals stack: each gets its own undo entry and its own expiry clock, so removing a
  second playlist does not shorten the first one's window. Rows and undo entries are slotted by
  **playlist id**, the order `customListPlaylists` guarantees (`ORDER BY p.id` — load-bearing:
  without it, GROUP BY order is optimizer-chosen and any refetch could reshuffle the list under
  the offers).
  Undo re-adds by `playlist_id` rather than the DELETE being deferred, because a deferred delete is
  lost outright if the tab closes inside the window, and because re-adding is exact: removal drops
  only the `user_playlist` row, so `customAddPlaylist`'s `playlist_id` branch re-attaches the same
  playlist without re-syncing YouTube, and nothing the parent authored lives on that join row.
  There is no separate reward-video list: per-playlist counts plus the drill-down carry everything
  it showed.
- **`DeleteAccountView`** — the last card in the grid: self-service account deletion, confirmed by
  the shared `PinConfirmModal` re-asking for the PIN (`DELETE /users/:auth0_id`). Its hint copy
  spells out what is deleted, what is retained, and that deletion is irreversible. It is the only
  red-button surface on the page, which is why this page re-neutralises the shared modal's action
  row (`.pin-confirm-modal-actions` in `settings.scss`) — the modal lives outside the page's
  green-button default. Semantics — what is purged, what is retained, the server-side PIN check —
  are owned by [accounts.md](accounts.md); the modal shape is documented on `/style-guide`.

## Save feedback

Every control saves on change, and `postSettings` **throws** on a non-2xx or network failure so the
result is visible. `useSaveState` drives a per-card indicator (`saving` → `saved`, auto-clearing,
or `error`), and the error state offers a retry that replays the last attempt. The indicator sits
on the card that changed, so a failure is attached to the control that caused it.

## Invariants

1. **Server is authoritative.** Both client rules here are mirrors. The bitmap dependency rules also
   live in server validation, and `MaxDiffForBitmap` is the source of truth for the ceiling. API
   clients bypassing the UI degrade gracefully — the server clamps and re-validates on save.
2. **`maxDiffForBitmap`/`minDiffForBitmap` ⇔ `MaxDiffForBitmap`/`MinDiffForBitmap` lockstep.** Any
   change to the server band formulas, the multiplier set, `MaxChainLen`, `MaxWordChainLen`,
   `MinConstructibleOperand`, or `LargeMaxOperand` must be reflected here in the same PR, or the
   slider range diverges from the server clamp. Mechanically guarded by the generated fixtures
   (`web/src/difficulty_band_fixtures.json`, `make gen-difficulty-fixtures`): the Go side pins the
   fixtures to the formulas (`TestDifficultyBandFixturesSync`, server/api), and
   `bitmap_validation.test.js` pins this mirror to the same fixtures at full float precision — a
   formula change fails the Go test until regenerated, and the regenerated fixtures fail the JS
   test until the mirror follows.
3. **The saved bitmap is always valid OR not saved.** `commit` POSTs only when `validateBitmap`
   passes.
4. **Dependent bits never outlive their parent.** Render-time gating plus `applyToggleRules`
   guarantee every `dependsOn` edge holds in the composed bitmap. Both halves read the same
   `dependsOn` declaration, so a pair cannot be gated on screen but unenforced in the bit math.

## Gotchas

- **The meter-percent denominator relies on `lo < hi`.** The percent computation divides by
  `ceiling - floor`. `targetDifficultyRange` collapses `lo` to `hi` when a degenerate envelope would
  invert the band (mirroring the server), which keeps the clamp safe but would make the denominator
  0 in that (currently unreachable) case; every valid envelope's floor sits strictly below its
  ceiling (property-tested server-side in `TestTargetDifficultyRange`).
- **Card placement is hand-maintained.** `PROBLEM_TYPE_GROUPS` is not derived from the enum; a new
  bit can be computed but invisible until placed. Error placement and the toggle math are derived
  from it, so those follow on their own once the bit is placed.

## Related files

- `web/src/settings.js` — `PROBLEM_TYPE_GROUPS`, `CROSS_CARD_REQUIREMENTS`, `REQUIREMENTS`,
  `closure`, `applyToggleRules`, `isParent`, `errorCardTitle`, `ProblemTypesSettingsView`,
  `TargetDifficultySettingsView`, `PlaylistsSettingsView`, `PlaylistRow`,
  `SettingsCard`, `useSaveState`, `MIN_PLAYABLE_VIDEOS`, `SettingsView`, `postSettings`,
  `DeleteAccountView`.
- `web/src/api.js` — `apiFetch`, which issues every call on this screen (settings save, the
  playlist list/add/remove/undo, account deletion). `PlaylistRow` takes `token` and calls it
  directly rather than being handed a header builder. See [accounts.md](accounts.md).
- `server/api/custom_handlers.go` — `customListPlaylists` (returns `PlaylistWithCounts`),
  `customListPlaylistVideos` (the drill-down; the `user_playlist` join is its authorization).
- `web/src/bitmap_validation.js` — `validateBitmap`, `maxDiffForBitmap`, `minDiffForBitmap`,
  `targetDifficultyRange`, `MIN_TARGET_DIFFICULTY`.
- `web/src/difficulty_band_fixtures.json` — generated Go↔JS parity fixtures
  (`make gen-difficulty-fixtures`, `cmd/gen_difficulty_fixtures`).
- `web/src/enums.generated.js` — `ProblemTypes` bit constants, generated from
  `server/mathcore/problem_type.go`; never hand-edit.
- `server/mathcore/difficulty.go` — `MaxDiffForBitmap`, `MinDiffForBitmap`, `TargetDifficultyRange`,
  `MinTargetDifficulty`, `MaxChainLen`, `MaxWordChainLen`, `MinConstructibleOperand`,
  `LargeMaxOperand`, `SmallMaxOperand`, `MediumMaxOperand` (the authoritative copies).
- [`docs/problem-generation.md`](problem-generation.md) — bit semantics, difficulty formula, the
  ceiling rationale, the new-bit checklist.

## Extension checklist (adding a problem-type bit to the screen)

Walk the full new-bit checklist in problem-generation.md; the settings-screen touchpoints are:

1. `make build-api` to pick the new `server/mathcore/problem_type.go` bit up in `ProblemTypes`.
2. Place it in `PROBLEM_TYPE_GROUPS` — pick the card (verb / noun-kind / noun-size / framing), label
   (parent vocabulary), and `dependsOn` if it's part of a dependency pair (that one field gets the
   row placement, the gating, and the toggle math).
3. If it adds a dependency rule: add a check + error code to `validateBitmap` with the `bits` the
   error concerns. Placement and toggle clearing are derived — a rule whose two bits sit on
   different cards needs a `CROSS_CARD_REQUIREMENTS` edge, since `dependsOn` can't express it.
4. If it changes the difficulty formula: update `maxDiffForBitmap` in lockstep with the server
   `MaxDiffForBitmap`.
5. Update this doc and its anchor block (the doc-sync test fails CI otherwise).
