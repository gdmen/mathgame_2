-- Percent notation cutover: a percent literal exists only as the left factor
-- of an "n% of X" multiplication (the `of` connective lexes as
-- multiplication). Re-run-safe passes over the pool plus a settings backfill,
-- pattern-based rather than id-based so rows generated between the analysis
-- snapshot and the deploy are covered too.
--
-- Pass 1: rewrite left-factor percent multiplications to the new connective
-- in both columns. symbolic_expression holds the canonical grammar
-- ("50% * 3250" becomes "50% of 3250"). expression holds the display skin
-- for non-WORD rows ("\% \times " becomes the "\%\text{ of }" skin, exactly
-- what DisplayExpression emits) and is prose for WORD rows, where the
-- replacement no-ops. Same tree, so bits, difficulty, and answer are
-- untouched. Idempotent: after one run the old pattern no longer occurs.
UPDATE problems SET
  symbolic_expression = REPLACE(symbolic_expression, '% * ', '% of '),
  expression = REPLACE(expression, '\\% \\times ', '\\%\\text{ of }')
  WHERE INSTR(COALESCE(symbolic_expression, ''), '% * ') > 0;

-- Pass 2: retire active rows whose percent shape the strict grammar cannot
-- express or that heuristic_2.1 no longer emits. Three clauses:
--   1. a percent NOT followed by the connective (additive percents
--      "40% + 10%", percent divisors "43 / 50%", trailing percents),
--   2. a percent-of PRECEDED by a multiplicative operator - "12 * 25% of 2"
--      or a "25% of 1 of..." chain - which the lexer rejects because the
--      percent would left-associate into a right-factor position,
--   3. a degenerate "n% of 1" factor in any position (a multiplicative
--      identity adds no operation to perform).
UPDATE problems SET status = 'deprecated'
  WHERE status = 'active'
    AND ( INSTR(REPLACE(COALESCE(symbolic_expression, ''), '% of ', ''), '%') > 0
          OR COALESCE(symbolic_expression, '') REGEXP '(\\*|÷|of) [0-9.]+% of '
          OR COALESCE(symbolic_expression, '') REGEXP '% of 1([^0-9.]|$)' );

-- Settings backfill: PERCENTAGES now requires MULTIPLICATION and
-- MEDIUM_NUMBERS (bit values 32768, 4, 128 - problem_type.go). The old
-- settings UI offered percentages with no dependency, so orphaned envelopes
-- can exist. Clear the orphan bit - exactly what the UI's applyToggleRules
-- would do - otherwise those users see a permanent validation error that
-- blocks every settings save, while the bit is already inert (the ceiling
-- and the builder both gate percent the same way).
UPDATE settings SET problem_type_bitmap = problem_type_bitmap & ~32768
  WHERE (problem_type_bitmap & 32768) > 0
    AND ((problem_type_bitmap & 4) = 0 OR (problem_type_bitmap & 128) = 0);
