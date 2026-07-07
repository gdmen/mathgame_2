// Part of the problem-generation system - documented in docs/problem-generation.md.
// Behavior changes here (bits, formula, pipeline, masks) REQUIRE updating that
// doc in the same PR. Formula changes also require a DifficultyVersion bump.
// Package api contains api routes, handlers, and models
package api // import "garydmenezes.com/mathgame/server/api"

import (
	"errors"
	"fmt"
	"hash/fnv"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/golang/glog"

	heuristic_generator "garydmenezes.com/mathgame/server/generator"
	llm_generator "garydmenezes.com/mathgame/server/llm_generator"
	"garydmenezes.com/mathgame/server/mathcore"
)

const (
	// recencyWindow is the base unit for recency-related selection sizes.
	recencyWindow = 50

	// recentProblemHistorySize is how many recent problems to hard-exclude.
	// Used by process_events.go to build prevIds.
	recentProblemHistorySize = recencyWindow

	// minSelectionPool is the smallest healthy candidate-pool size; below
	// this we trigger background generation (non-blocking) to refill.
	minSelectionPool = 2 * recencyWindow

	// recentlyShownProblemsTrimSize is the max rows per user retained in the
	// recently_shown_problems table. The async trim job evicts anything
	// older than this many shown_at-DESC entries; evicted problems become
	// "never shown" to the recency-bias sort and re-enter the rotation.
	recentlyShownProblemsTrimSize = 4 * recencyWindow

	// lruTopFrac is the fraction of the recency-sorted pool we pick from
	// uniformly at random. With minSelectionPool=100 and 0.20 → top 20.
	lruTopFrac = 0.20

	// problemSelectionEpsilon: candidate difficulty must be within this
	// additive window of the user's target_difficulty (target ± epsilon).
	problemSelectionEpsilon = 1.5
)

// formatUintsForSQLIn formats a slice of unsigned integers as "1,2,3" for use in a SQL "IN (...)" clause.
func formatUintsForSQLIn[T ~uint32 | ~uint64](vals []T) string {
	parts := make([]string, len(vals))
	for i, v := range vals {
		parts[i] = strconv.FormatUint(uint64(v), 10)
	}
	return strings.Join(parts, ",")
}

// Get all problem ids that satisfy this ProblemTypeBitmap and have similar
// Difficulty. Bitwise-subset selection: a problem matches iff every bit it
// carries is enabled in the user's settings -
// (problem_type_bitmap & ~enabled) = 0.
//
// problem_type_bitmap != 0 is defense-in-depth: a zero bitmap is a subset of
// everything, so an unstamped row would be served to every user. The
// backfill census flags such rows for review; this clause keeps them out of
// selection regardless.
//
// An enabled bit means that feature MAY be served, never that it MUST be.
func (a *Api) getSatisfyingProblemIds(logPrefix string, settings *Settings, prevIds *[]uint32) (*[]uint32, error) {
	diffLowerBound := settings.TargetDifficulty - problemSelectionEpsilon
	diffUpperBound := settings.TargetDifficulty + problemSelectionEpsilon
	clause := fmt.Sprintf("(problem_type_bitmap & ~%d) = 0 AND problem_type_bitmap != 0 AND difficulty >= %g AND difficulty <= %g AND status='active'",
		settings.ProblemTypeBitmap,
		diffLowerBound,
		diffUpperBound,
	)
	if len(*prevIds) > 0 {
		clause = fmt.Sprintf("id NOT IN (%s) AND ", formatUintsForSQLIn(*prevIds)) + clause
	}
	return a.newestVersionTier(logPrefix, clause)
}

// newestVersionTier runs the satisfying-set query for whereClause and returns
// the ids of the highest-ranked generator version present (see generatorRank),
// so selection prefers newer generators and falls back to an older version only
// when no newer version matches the envelope + difficulty window.
func (a *Api) newestVersionTier(logPrefix string, whereClause string) (*[]uint32, error) {
	query := "SELECT id, generator FROM problems WHERE " + whereClause
	glog.Infof("%s newestVersionTier: %s", logPrefix, query)
	rows, err := a.DB.Query(query)
	if err != nil {
		glog.Errorf("%s newestVersionTier query: %v", logPrefix, err)
		return nil, err
	}
	defer rows.Close()

	byRank := map[int][]uint32{}
	bestRank := 0
	haveBest := false
	for rows.Next() {
		var id uint32
		var generator string
		if err := rows.Scan(&id, &generator); err != nil {
			continue
		}
		r := generatorRank[generator]
		byRank[r] = append(byRank[r], id)
		if !haveBest || r > bestRank {
			bestRank = r
			haveBest = true
		}
	}
	ids := byRank[bestRank]
	if ids == nil {
		ids = []uint32{}
	}
	return &ids, nil
}

// pickPooledProblem picks one id from pids (recency-biased) and fetches it,
// the shared pooled-pick for selectProblem's default and recency-relaxed
// stages. Returns nil on an empty pool or a fetch error — the error is logged,
// never written to the response, so the caller can fall through to its next
// stage rather than failing the request.
func (a *Api) pickPooledProblem(logPrefix string, settings *Settings, pids []uint32) *Problem {
	if len(pids) == 0 {
		return nil
	}
	pid := a.pickWithRecencyBias(logPrefix, settings.UserId, pids)
	p, status, msg, err := a.problemManager.Get(pid)
	if err != nil || status != http.StatusOK {
		glog.Infof("%s unexpected (recoverable) error fetching problem (id=%d): %s : %v", logPrefix, pid, msg, err)
		return nil
	}
	return p
}

func (a *Api) selectProblem(logPrefix string, settings *Settings, prevIds *[]uint32) (*Problem, error) {
	// Check spaced repetition review queue first
	dueReviewID := a.getDueReviewProblem(logPrefix, settings)
	if dueReviewID != 0 {
		p, status, msg, err := a.problemManager.Get(dueReviewID)
		if err == nil && status == http.StatusOK && p.Status == StatusActive {
			glog.Infof("%s serving spaced rep review problem=%d", logPrefix, dueReviewID)
			return p, nil
		}
		glog.Infof("%s spaced rep problem=%d unavailable: %s %v", logPrefix, dueReviewID, msg, err)
	}

	pids, err := a.getSatisfyingProblemIds(logPrefix, settings, prevIds)
	if err != nil {
		return nil, err
	}

	// Background generation is WORD-only (the expensive LLM narration); kick it
	// off only when WORD is enabled. Non-WORD problems are cheap and generated
	// live below, so they need no background batch.
	if len(*pids) < minSelectionPool && settings.ProblemTypeBitmap&uint64(mathcore.WORD) != 0 {
		glog.Infof("%s generating new WORD problems because there are only %d matching", logPrefix, len(*pids))
		a.generateProblemsBackground(logPrefix, settings)
	}

	if p := a.pickPooledProblem(logPrefix, settings, *pids); p != nil {
		return p, nil
	}

	// No servable pooled problem. Generate a non-WORD problem live with the
	// in-process heuristic (cheap, no OpenAI) — this is both the immediate answer
	// and how non-WORD problems enter the pool (they get no background batch). For
	// a WORD envelope the background narration kicked off above fills the word
	// pool for subsequent requests; a heuristic non-WORD problem is served now as
	// the low-latency stopgap.
	inputProblemType := mathcore.ProblemType(settings.ProblemTypeBitmap)
	heuristicType := inputProblemType &^ mathcore.WORD
	if heuristicType != 0 {
		glog.Infof("%s no pooled problem; generating a heuristic one live", logPrefix)
		if p := a.runHeuristicGenerator(logPrefix, settings, 3, heuristicType); p != nil {
			return p, nil
		}
		// The heuristic minted nothing new: the difficulty band is saturated, so
		// every candidate collided with an existing row that the recency
		// exclusion had masked from the pooled pick above. Relax recency and
		// re-serve a pooled problem — a brief repeat beats failing the request.
		noRecency := []uint32{}
		if pids, err := a.getSatisfyingProblemIds(logPrefix, settings, &noRecency); err == nil {
			if p := a.pickPooledProblem(logPrefix, settings, *pids); p != nil {
				return p, nil
			}
		}
	}

	// Last resort: the envelope is WORD-only (no non-WORD skeleton to build), or
	// the heuristic produced nothing. Block on a synchronous WORD generation.
	glog.Infof("%s no heuristic-eligible types; blocking on WORD generation", logPrefix)
	return a.generateProblem(logPrefix, settings)
}

func (a *Api) generateProblem(logPrefix string, settings *Settings) (*Problem, error) {
	retries := 5
	var err error
	var p *Problem
	for i := 0; i < retries; i++ {
		p, err = a.generateProblems(logPrefix, settings, 5)
		if p != nil {
			return p, nil
		}
	}
	return nil, err
}

// backgroundGenLocks dedups concurrent background problem generations per
// user. Multiple rapid events (e.g., the 500ms working_on_problem ticker)
// can trigger generateProblemsBackground many times over a single slow LLM
// round-trip; without this guard, we stack up goroutines all trying to
// insert similar problems and pounding the DB + OpenAI quota.
//
// Key: user_id (uint32). Value: *sync.Mutex. TryLock wins the right to run;
// losers log and skip. The mutex is released in the goroutine's defer.
var backgroundGenLocks sync.Map

// backgroundGenFn is swappable for tests.
var backgroundGenFn = func(a *Api, logPrefix string, settings *Settings, numProblems int) {
	a.generateProblems(logPrefix, settings, numProblems)
}

// llmNarrateProblemFn and llmValidateProblemFn are seams for the LLM narration
// and validation calls. Production points them at llm_generator.NarrateProblems
// / ValidateWordProblem; tests override them to return canned prose and
// validation outcomes without hitting OpenAI.
var (
	llmNarrateProblemFn  = llm_generator.NarrateProblems
	llmValidateProblemFn = llm_generator.ValidateWordProblem
)

func (a *Api) generateProblemsBackground(logPrefix string, settings *Settings) error {
	userID := settings.UserId
	muAny, _ := backgroundGenLocks.LoadOrStore(userID, &sync.Mutex{})
	mu := muAny.(*sync.Mutex)
	if !mu.TryLock() {
		glog.Infof("%s background generation already running for user=%d; skipping", logPrefix, userID)
		return nil
	}

	// Detach from the request: make a local copy so the goroutine can't see
	// mutations the main request might make to settings after returning.
	settingsCopy := *settings

	go func() {
		defer mu.Unlock()
		defer func() {
			if r := recover(); r != nil {
				glog.Errorf("%s background generation panicked: %v", logPrefix, r)
			}
		}()
		backgroundGenFn(a, logPrefix, &settingsCopy, 20)
	}()

	return nil
}

// runHeuristicGenerator generates problems using the difficulty-targeting
// heuristic_2.0 builder. Supports every non-WORD bit and arbitrary stacks of
// them (the builder aims at the target difficulty within the envelope).
// WORD problems are generated via the LLM generator instead.
// Returns the last new problem created (nil if none).
//
// Does NOT take a gin.Context: this function may run in a background goroutine
// after the originating request has returned. Writing to a stale/reused context
// from a background path corrupts unrelated in-flight requests. Errors are
// logged via glog; callers decide how to handle a nil return.
func (a *Api) runHeuristicGenerator(logPrefix string, settings *Settings, numProblems int, problemType mathcore.ProblemType) *Problem {
	var newProblem *Problem
	if problemType&(mathcore.ADDITION|mathcore.SUBTRACTION|mathcore.MULTIPLICATION|mathcore.DIVISION) == 0 {
		return nil
	}
	// heuristic_2.0 is difficulty-targeting: it takes the envelope bitmap and the
	// user's target_difficulty directly and aims each candidate at it (the
	// magnitude bracket, chain length, and concept subset are derived internally).
	rng := rand.New(rand.NewSource(rand.Int63()))
	funnel := newGenerationFunnel(numProblems)
	for i := 0; i < numProblems; i++ {
		expr, answer, err := heuristic_generator.BuildProblem(problemType, settings.TargetDifficulty, rng)
		if err != nil {
			if _, ok := err.(*heuristic_generator.OptionsError); ok {
				glog.Errorf("%s Failed options validation: %v", logPrefix, err)
				return nil
			}
			glog.Errorf("%s Couldn't generate problem: %v", logPrefix, err)
			continue
		}
		funnel.returned++

		// Heuristic candidates pass the same admission pipeline as LLM
		// candidates: the generator is trusted to be well-formed, but the
		// pipeline is the single source of truth for stamping and envelope.
		adm := mathcore.AdmitExpression(expr)
		if adm.RejectStage != "" {
			funnel.reject(adm.RejectStage)
			glog.Infof("%s heuristic reject [%s]: %s (%q)", logPrefix, adm.RejectStage, adm.RejectWhy, expr)
			continue
		}
		if err := mathcore.VerifyAnswerSymbolic(adm.Tokens, answer); err != nil {
			funnel.reject(rejectAnswer)
			glog.Errorf("%s heuristic answer reject: %v (%q = %q)", logPrefix, err, expr, answer)
			continue
		}
		// Envelope is the problemType param (the caller-masked request for
		// THIS generation call, always a subset of the user's settings), not
		// settings.ProblemTypeBitmap directly. NormalizeProblemBitmap is a
		// no-op on the parser's own output (it co-sets these bits already);
		// applied for uniformity with the WORD path.
		bitmap := mathcore.NormalizeProblemBitmap(adm.Bitmap)
		if v := mathcore.EnvelopeViolation(bitmap, uint64(problemType)); v != "" {
			funnel.reject(rejectEnvelope)
			glog.Infof("%s heuristic envelope reject [%s]: %q", logPrefix, v, expr)
			continue
		}

		model := &Problem{}
		model.Generator = heuristic_generator.VERSION
		// adm.Expr is the canonical grammar form (unspaced a/b fractions); it is
		// the machine form (scored, answer-checked, and a word generator's
		// prose feedstock), stored in symbolic_expression. expression carries the
		// \frac display skin so a fraction under division renders unambiguously.
		model.Expression = mathcore.DisplayExpression(adm.Expr)
		model.SymbolicExpression = adm.Expr
		model.Answer = answer
		model.ProblemTypeBitmap = bitmap
		// Stored difficulty is a function of the problem itself, not the
		// requester's target - the pool is shared across users. Scored from the
		// grammar form; the \frac display carries no \text{}, so it is not a
		// word problem and takes no word bonus.
		model.Difficulty = mathcore.ComputeProblemDifficulty(model.Expression, model.SymbolicExpression)
		model.DifficultyVersion = mathcore.DifficultyVersion
		glog.Infof("%s heuristic problem: %s = %s (computed_diff=%g bitmap=%d)", logPrefix, model.Expression, model.Answer, model.Difficulty, model.ProblemTypeBitmap)
		if !a.storeGeneratedProblem(logPrefix, "heuristic", model, funnel) {
			continue
		}
		newProblem = model
	}
	glog.Infof("%s heuristic %s", logPrefix, funnel)
	return newProblem
}

// storeGeneratedProblem hashes the model's expression into its id, rejects a
// hash collision with an existing row, and inserts it — the shared tail of both
// generators. Updates the funnel and returns true iff the row was created.
func (a *Api) storeGeneratedProblem(logPrefix, kind string, model *Problem, funnel *generationFunnel) bool {
	h := fnv.New32a()
	h.Write([]byte(model.Expression))
	model.Id = h.Sum32()
	if _, status, _, _ := a.problemManager.Get(model.Id); status != http.StatusNotFound {
		funnel.reject(rejectCollision)
		return false
	}
	if status, msg, err := a.problemManager.Create(model); err != nil {
		funnel.reject(rejectCreate)
		glog.Errorf("%s could not create %s problem (%d: %s): %v", logPrefix, kind, status, msg, err)
		return false
	}
	funnel.inserted++
	return true
}

// generateProblems is the batched generation routine for the background refill
// (and the WORD-only synchronous last resort). It generates WORD problems ONLY:
// the LLM narration is the expensive part worth pre-generating into the shared
// pool. Non-WORD problems are cheap (the in-process heuristic) and generated live
// on the request path (selectProblem), so they are never batched here. It does
// NOT take a gin.Context: in the background path we must not share a context with
// the originating request (gin pools contexts and they get reused by later
// requests, so writes to a stale context corrupt unrelated in-flight responses).
// Errors are logged via glog; callers decide how to handle a nil return.
func (a *Api) generateProblems(logPrefix string, settings *Settings, numProblems int) (*Problem, error) {
	if settings.ProblemTypeBitmap == 0 {
		return nil, errors.New("settings.ProblemTypeBitmap is empty. Cannot generate problems.")
	}
	inputProblemType := mathcore.ProblemType(settings.ProblemTypeBitmap)
	nonWordType := inputProblemType &^ mathcore.WORD

	// WORD only. A skeleton needs a non-WORD envelope to build from; the settings
	// UI's core-op rule makes WORD imply nonWordType != 0. A WORD-only envelope
	// from an API client that bypasses that rule yields nothing (a real word
	// problem stamps a core-op bit its envelope lacks and is rejected anyway).
	if inputProblemType&mathcore.WORD == 0 || nonWordType == 0 {
		return nil, errors.New("generateProblems: nothing to batch (WORD-only; non-WORD is generated live)")
	}
	if p := a.runWordGenerator(logPrefix, settings, numProblems, nonWordType); p != nil {
		return p, nil
	}
	return nil, errors.New("Failed to produce any valid new WORD problem.")
}

// runWordGenerator builds scored heuristic skeletons, has the LLM narrate each
// into prose, validates the narration against its skeleton (answer + form, via
// a secondary model), and stores the survivors — the inversion of the old
// "author a word problem, then reverse-engineer its symbolic form" flow.
// Difficulty is skeleton x the word concept by construction. nonWordType is the
// envelope with WORD stripped: the skeleton source.
//
// Does NOT take a gin.Context: it may run in a background goroutine. Errors are
// logged via glog; a nil return means no word problem was produced.
func (a *Api) runWordGenerator(logPrefix string, settings *Settings, numProblems int, nonWordType mathcore.ProblemType) *Problem {
	// Aim the skeleton so the narrated word problem lands near the target: the
	// word concept multiplies raw difficulty by ConceptWord, so build at the
	// target raw budget divided by that factor.
	rawTarget := mathcore.RawForDifficulty(settings.TargetDifficulty) / mathcore.ConceptWord
	envelopeFeatures := mathcore.ProblemTypeToFeatures(nonWordType)

	rng := rand.New(rand.NewSource(rand.Int63()))
	var skeletons []llm_generator.Skeleton
	for i := 0; i < numProblems; i++ {
		expr, answer, err := heuristic_generator.BuildWordSkeletonRaw(nonWordType, rawTarget, rng)
		if err != nil {
			glog.Errorf("%s word skeleton build failed: %v", logPrefix, err)
			continue
		}
		adm := mathcore.AdmitExpression(expr)
		if adm.RejectStage != "" {
			glog.Infof("%s word skeleton reject [%s]: %q", logPrefix, adm.RejectStage, expr)
			continue
		}
		if err := mathcore.VerifyAnswerSymbolic(adm.Tokens, answer); err != nil {
			glog.Errorf("%s word skeleton answer reject: %v (%q = %q)", logPrefix, err, expr, answer)
			continue
		}
		skeletons = append(skeletons, llm_generator.Skeleton{
			SymbolicExpression: adm.Expr,
			Answer:             answer,
			Features:           envelopeFeatures,
		})
	}
	if len(skeletons) == 0 {
		return nil
	}

	narrated, err := llmNarrateProblemFn(skeletons)
	if err != nil {
		glog.Errorf("%s word narration failed: %v", logPrefix, err)
		return nil
	}

	funnel := newGenerationFunnel(numProblems)
	funnel.returned = len(narrated)
	var newProblem *Problem
	for i := range narrated {
		p := narrated[i]
		glog.Infof("%s narrated problem: %v", logPrefix, p)

		// Admit the prose for its canonical stored form and to confirm word-ness
		// (the WORD bit below); its bits do NOT feed the stamp - the skeleton does.
		adm := mathcore.AdmitExpression(p.Expression)
		if adm.RejectStage != "" {
			funnel.reject(adm.RejectStage)
			glog.Infof("%s narration reject [%s]: %q", logPrefix, adm.RejectStage, p.Expression)
			continue
		}
		if adm.Bitmap&uint64(mathcore.WORD) == 0 {
			// The narrator returned bare symbolic text, not a story.
			funnel.reject(rejectValidator)
			glog.Infof("%s narration is not a word problem: %q", logPrefix, p.Expression)
			continue
		}

		// Secondary-model validation: solve the prose (its answer must match the
		// skeleton's) and confirm the prose poses the skeleton (form check).
		if err := llmValidateProblemFn(&p); err != nil {
			funnel.reject(rejectValidator)
			glog.Infof("%s narration validator reject: %v", logPrefix, err)
			continue
		}

		// The skeleton owns the math: re-admit it for its canonical form and
		// shape bits, and confirm it still answers correctly.
		admSym := mathcore.AdmitExpression(p.SymbolicExpression)
		if admSym.RejectStage != "" {
			funnel.reject(admSym.RejectStage)
			glog.Infof("%s word skeleton reject [%s]: %q", logPrefix, admSym.RejectStage, p.SymbolicExpression)
			continue
		}
		if err := mathcore.VerifyAnswerSymbolic(admSym.Tokens, p.Answer); err != nil {
			funnel.reject(rejectAnswer)
			glog.Errorf("%s word skeleton answer reject: %v (%q = %q)", logPrefix, err, p.SymbolicExpression, p.Answer)
			continue
		}

		// Stamp from the SKELETON, not the prose: WordFormBitmap (WORD plus
		// the skeleton's own shape bits, minus PEMDAS). The prose supplies
		// word-ness and its canonical stored form only - admitting it for
		// bits would re-derive magnitude from incidental story numerals (a
		// "200 seats" story stamping LARGE_NUMBERS onto a small skeleton),
		// which then trips the envelope check and drops a valid narration.
		// The skeleton is the source of truth for the math and the bits.
		bitmap := mathcore.WordFormBitmap(admSym.Bitmap)
		if v := mathcore.EnvelopeViolation(bitmap, settings.ProblemTypeBitmap); v != "" {
			funnel.reject(rejectEnvelope)
			glog.Infof("%s narration envelope reject [%s]: %q", logPrefix, v, p.Expression)
			continue
		}

		model := &Problem{}
		model.Generator = llm_generator.VERSION
		model.ProblemTypeBitmap = bitmap
		model.Expression = adm.Expr
		model.SymbolicExpression = admSym.Expr
		model.Answer = p.Answer
		model.Explanation = RewriteLetterInProse(p.Explanation, adm.RewroteLetter)
		// Scored from the skeleton with the word concept applied (the prose
		// carries the \text{} that fires the word bonus): skeleton x word.
		model.Difficulty = mathcore.ComputeProblemDifficulty(adm.Expr, admSym.Expr)
		model.DifficultyVersion = mathcore.DifficultyVersion
		glog.Infof("%s word problem: %s (symbolic=%q computed_diff=%g bitmap=%d)", logPrefix, model.Expression, model.SymbolicExpression, model.Difficulty, model.ProblemTypeBitmap)

		if !a.storeGeneratedProblem(logPrefix, "word", model, funnel) {
			continue
		}
		newProblem = model
	}
	glog.Infof("%s word %s", logPrefix, funnel)
	return newProblem
}
