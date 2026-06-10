# Migration 39 — EXPLAIN ANALYZE artifact

Bitwise-subset selection (`(problem_type_bitmap & ~enabled) = 0`, #225 PR2)
measured on a **320,000-row synthetic copy** of the problems table (MySQL
8.0.26, local). Synthetic distribution: one core-op bit per row plus
realistic magnitude/concept bit mixes (~33% MEDIUM, ~6% LARGE, 20% CHAINED,
14% MISSING, 9% FRACTIONS, 8% WORD), difficulty uniform 1.0–25.0, 10%
disabled — built by the script inline in the PR discussion; rebuildable from
this description.

Query under test (the Stage-1/2 selection shape; enabled bitmap = 1695 =
ADD|SUB|MUL|DIV|FRACTIONS|MEDIUM|CHAINED|MISSING, difficulty window 8.5–11.5):

```sql
SELECT id FROM problems
WHERE (problem_type_bitmap & ~1695) = 0
  AND problem_type_bitmap != 0
  AND difficulty >= 8.5 AND difficulty <= 11.5
  AND disabled = 0;
```

## Result: the index must COVER the bitmap column

| Plan | actual time (320K rows) |
|---|---|
| **`(disabled, difficulty, problem_type_bitmap)` — migration 39's index** | **~20 ms** |
| old 4-col `(disabled, difficulty, grade_level, problem_type_bitmap)` | ~20 ms (also covering; carries a dead column) |
| plain `(disabled, difficulty)` | ~130 ms — index narrows the range but every surviving row needs a **row lookup** to test the bitmap |
| no index (full scan) | ~132 ms |

A bitwise expression can't be index-*seeked*, but it can be evaluated from
index *pages*: with `problem_type_bitmap` as the trailing index column the
filter runs covering, no row fetches. Without it, the plain 2-column index is
barely better than a full scan at this selectivity (the difficulty window
keeps ~12% of rows). Hence `(disabled, difficulty, problem_type_bitmap)`.

## Chosen plan (verbatim)

```
-> Filter: ((problems.disabled = 0) and ((problems.problem_type_bitmap & <cache>(~(1695))) = 0)
           and (problems.problem_type_bitmap <> 0)
           and (problems.difficulty >= 8.5) and (problems.difficulty <= 11.5))
   (cost=14743.15 rows=65502) (actual time=0.031..19.755 rows=32431 loops=1)
    -> Index range scan on problems using idx_problems_disabled_diff_bitmap
       (cost=14743.15 rows=72780) (actual time=0.027..9.976 rows=37329 loops=1)
```

Topic-filtered variant (`AND (problem_type_bitmap & 1024) != 0`,
`getSatisfyingProblemIdsForTopic`): same plan + one more index-page filter
term, ~18 ms, 4,623 rows.

Plain (disabled, difficulty) plan for comparison:

```
-> Filter: (((problems.problem_type_bitmap & <cache>(~(1695))) = 0) and (problems.problem_type_bitmap <> 0))
   (actual time=0.508..130.413 rows=32431 loops=1)
    -> Index range scan on problems using idx_problems_disabled_difficulty, with index condition:
       ((problems.disabled = 0) and (problems.difficulty >= 8.5) and (problems.difficulty <= 11.5))
       (actual time=0.499..120.143 rows=37329 loops=1)
```

Baseline reference: the pre-migration-37 behavior this guards against was a
3.5 s full-table scan in prod (#190).
