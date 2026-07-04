-- Retire every generator version older than the current pair: heuristic_2.0
-- (the sole non-WORD source) and llm_0.6 (WORD narration). Migration 46 step 4
-- already retired heuristic_0.0, llm_0.0, llm_0.1, and llm_0.2. This extends the
-- cutover to the remaining pre-current versions now that heuristic_2.0
-- supersedes heuristic_1.0 and llm_0.6 supersedes the llm_0.3 through llm_0.5
-- line. Deprecated rows are valid history (still counted in metrics, since they
-- were legitimate when answered) but are no longer served.
--
-- Matches NOT IN the current pair rather than an explicit legacy list, so any
-- un-enumerated legacy generator string is retired too and only the two current
-- versions stay servable. Touches only rows still 'active', so an event-flagged
-- ('reported') or already-'deprecated' row keeps its status, and a re-run
-- no-ops (the status filter excludes rows already moved off 'active').
UPDATE problems SET status = 'deprecated'
  WHERE generator NOT IN ('heuristic_2.0', 'llm_0.6') AND status = 'active';
