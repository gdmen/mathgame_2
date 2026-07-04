package api

// Problem lifecycle states, stored in problems.status. See docs/schema.md for
// the full semantics (served? / counts in metrics? / who sets it).
const (
	// StatusActive: served and counted. The insert default.
	StatusActive = "active"
	// StatusDeprecated: not served, still counted — valid when shown, but its
	// generator version or format is retired. Set only by the migration
	// backfill; never at runtime.
	StatusDeprecated = "deprecated"
	// StatusReported: not served, still counted — an unvalidated bad-problem
	// claim from bad_problem_system (KaTeX render failure) or bad_problem_user
	// (parent report). Set at runtime by the event write path.
	StatusReported = "reported"
	// StatusIncorrect: not served — a human/admin has confirmed the problem is
	// wrong. Reserved for a future admin-review flow; nothing sets it today.
	StatusIncorrect = "incorrect"
)
