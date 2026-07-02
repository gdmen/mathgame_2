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

	"github.com/gin-gonic/gin"
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
	clause := fmt.Sprintf("(problem_type_bitmap & ~%d) = 0 AND problem_type_bitmap != 0 AND difficulty >= %g AND difficulty <= %g AND disabled=0",
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

func (a *Api) selectProblem(logPrefix string, c *gin.Context, settings *Settings, prevIds *[]uint32) (*Problem, error) {
	// Check spaced repetition review queue first
	dueReviewID := a.getDueReviewProblem(logPrefix, settings)
	if dueReviewID != 0 {
		p, status, msg, err := a.problemManager.Get(dueReviewID)
		if err == nil && status == http.StatusOK && !p.Disabled {
			glog.Infof("%s serving spaced rep review problem=%d", logPrefix, dueReviewID)
			return p, nil
		}
		glog.Infof("%s spaced rep problem=%d unavailable: %s %v", logPrefix, dueReviewID, msg, err)
	}

	pids, err := a.getSatisfyingProblemIds(logPrefix, settings, prevIds)
	if err != nil {
		return nil, err
	}

	if len(*pids) < minSelectionPool {
		glog.Infof("%s generating new problems because there are only %d problems", logPrefix, len(*pids))
		a.generateProblemsBackground(logPrefix, settings)
	}

	if len(*pids) > 0 {
		pid := a.pickWithRecencyBias(logPrefix, settings.UserId, *pids)
		p, status, msg, err := a.problemManager.Get(pid)
		if HandleMngrResp(logPrefix, c, status, msg, err, p) != nil {
			glog.Infof("%s unexpected (recoverable) error fetching problem (id=%d): %s : %s", logPrefix, pid, msg, err)
		} else {
			return p, nil
		}
	}

	// Pool is empty. The LLM backfill was already kicked off above via
	// generateProblemsBackground; we don't want the user waiting on an
	// OpenAI request here. Serve a heuristic-generated problem synchronously
	// so they see something immediately. The LLM backfill fills the pool for
	// subsequent requests.
	inputProblemType := mathcore.ProblemType(settings.ProblemTypeBitmap)
	heuristicType := inputProblemType &^ mathcore.WORD
	if heuristicType != 0 {
		glog.Infof("%s pool empty; serving heuristic problem while LLM backfills", logPrefix)
		p, _, _ := a.runHeuristicGenerator(logPrefix, settings, 3, heuristicType)
		if p != nil {
			return p, nil
		}
	}

	// Last resort: the user's enabled types are WORD-only (or the heuristic
	// couldn't produce one). Block on a synchronous LLM call.
	glog.Infof("%s pool empty and no heuristic-eligible types; blocking on LLM", logPrefix)
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

// llmGenerateProblemFn and llmValidateProblemFn are seams for the LLM
// problem-generation and validation calls. Production points them at
// llm_generator.GenerateProblem / ValidateWordProblem; tests override them
// to return canned problems and validation outcomes without hitting OpenAI.
var (
	llmGenerateProblemFn = llm_generator.GenerateProblem
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
// Returns the last new problem created, the count of new problems, and the set
// of unique IDs.
//
// Does NOT take a gin.Context: this function may run in a background goroutine
// after the originating request has returned. Writing to a stale/reused context
// from a background path corrupts unrelated in-flight requests. Errors are
// logged via glog; callers decide how to handle a nil return.
func (a *Api) runHeuristicGenerator(logPrefix string, settings *Settings, numProblems int, problemType mathcore.ProblemType) (*Problem, int, map[uint32]bool) {
	uniqueIds := map[uint32]bool{}
	newCount := 0
	var newProblem *Problem
	if problemType&(mathcore.ADDITION|mathcore.SUBTRACTION|mathcore.MULTIPLICATION|mathcore.DIVISION) == 0 {
		return nil, 0, uniqueIds
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
				return nil, newCount, uniqueIds
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
		h := fnv.New32a()
		h.Write([]byte(model.Expression))
		model.Id = h.Sum32()
		_, getStatus, _, _ := a.problemManager.Get(model.Id)
		if getStatus != http.StatusNotFound {
			funnel.reject(rejectCollision)
			continue
		}
		uniqueIds[model.Id] = true
		status, msg, err := a.problemManager.Create(model)
		if err != nil {
			funnel.reject(rejectCreate)
			glog.Errorf("%s could not create heuristic problem (%d: %s): %v", logPrefix, status, msg, err)
			continue
		}
		funnel.inserted++
		newCount++
		newProblem = model
	}
	glog.Infof("%s heuristic %s", logPrefix, funnel)
	return newProblem, newCount, uniqueIds
}

// generateProblems is the core generation routine used by both the sync
// request path and the background goroutine. It does NOT take a gin.Context:
// in the background path we must not share a context with the originating
// request (gin pools contexts and they get reused by later requests, so
// writes to a stale context corrupt unrelated in-flight responses).
// Errors are logged via glog; callers decide how to handle a nil return.
func (a *Api) generateProblems(logPrefix string, settings *Settings, numProblems int) (*Problem, error) {
	var model *Problem
	var newProblem *Problem
	if settings.ProblemTypeBitmap == 0 {
		return nil, errors.New("settings.ProblemTypeBitmap is empty. Cannot generate problems.")
	}
	uniqueIds := map[uint32]bool{}
	newCount := 0
	inputProblemType := mathcore.ProblemType(settings.ProblemTypeBitmap)
	// Try the LLM generator first. It produces richer content (word problems,
	// varied phrasings) and should be the primary
	// source for every problem type it can handle. The heuristic generator is
	// a deterministic, offline fallback for when OpenAI is unreachable or
	// returns no valid problems.
	{
		constraints := mathcore.BuildBitConstraints(inputProblemType)
		generatorOpts := &llm_generator.Options{
			Features:         mathcore.ProblemTypeToFeatures(inputProblemType),
			TargetDifficulty: settings.TargetDifficulty,
			NumProblems:      numProblems, // we still return just one problem, but this lets us reduce the number of OpenAI calls we need to make
			Constraints:      constraints,
		}
		var err error
		var generatorProblems []llm_generator.Problem
		generatorProblems, err = llmGenerateProblemFn(generatorOpts)
		if err != nil {
			// Fall back to heuristic when OpenAI fails. Strip WORD since the
			// heuristic doesn't produce word problems, and fall back on the
			// remaining arithmetic types.
			heuristicType := inputProblemType &^ mathcore.WORD
			if heuristicType != 0 {
				glog.Infof("%s OpenAI failed (%v), falling back to heuristic generator", logPrefix, err)
				newProblem, newCount, uniqueIds = a.runHeuristicGenerator(logPrefix, settings, numProblems, heuristicType)
			} else {
				msg := "Couldn't generate problems"
				glog.Errorf("%s %s: %v", logPrefix, msg, err)
				return nil, err
			}
		} else {
			funnel := newGenerationFunnel(numProblems)
			funnel.returned = len(generatorProblems)
			for _, p := range generatorProblems {
				glog.Infof("%s generated problem: %v", logPrefix, p)

				// Admission pipeline: normalize -> lex -> rewrite ->
				// detect -> unknown rules. The LLM's self-reported features
				// are NOT trusted for stamping.
				adm := mathcore.AdmitExpression(p.Expression)
				if adm.RejectStage != "" {
					funnel.reject(adm.RejectStage)
					glog.Infof("%s LLM reject [%s]: %s (%q)", logPrefix, adm.RejectStage, adm.RejectWhy, p.Expression)
					continue
				}
				bitmap := adm.Bitmap

				// symbolicExpr is the bare computation a WORD problem asks for,
				// set from the validated symbolic_expression below; empty for
				// symbolic problems, whose Expression is already the computation.
				symbolicExpr := ""

				// Local-first validation: symbolic problems are verified by
				// the exact evaluator with zero LLM calls; WORD problems get
				// one validator round-trip that checks the answer, judges
				// envelope compliance against the same constraints the
				// generator saw, and extracts topic features for stamping.
				if bitmap&uint64(mathcore.WORD) == 0 {
					if err := mathcore.VerifyAnswerSymbolic(adm.Tokens, p.Answer); err != nil {
						funnel.reject(rejectAnswer)
						glog.Infof("%s LLM answer reject: %v (%q = %q)", logPrefix, err, adm.Expr, p.Answer)
						continue
					}
				} else {
					features, err := llmValidateProblemFn(&p, constraints, mathcore.ValidatorFeatureNames)
					if err != nil {
						funnel.reject(rejectValidator)
						glog.Infof("%s LLM validator reject: %v", logPrefix, err)
						continue
					}
					// Validator-extracted topic bits stamp the WORD problem;
					// parser-derived shape bits (magnitude, chained, word)
					// are already in the bitmap.
					bitmap |= uint64(mathcore.FeaturesToProblemType(features))

					// The symbolic_expression is the bare computation the word
					// problem asks for; its difficulty is scored from this. Trust
					// it only after it lexes and evaluates to the stated answer.
					if p.SymbolicExpression != "" {
						admSym := mathcore.AdmitExpression(p.SymbolicExpression)
						if admSym.RejectStage != "" {
							funnel.reject(admSym.RejectStage)
							glog.Infof("%s LLM symbolic_expression reject [%s]: %q", logPrefix, admSym.RejectStage, p.SymbolicExpression)
							continue
						}
						if err := mathcore.VerifyAnswerSymbolic(admSym.Tokens, p.Answer); err != nil {
							funnel.reject(rejectAnswer)
							glog.Infof("%s LLM symbolic_expression answer reject: %v (%q = %q)", logPrefix, err, admSym.Expr, p.Answer)
							continue
						}
						symbolicExpr = admSym.Expr
						// The form reveals the true computation shape (operations,
						// magnitude, chaining) the prose hides. Stamp those bits so
						// the bitmap matches the difficulty scored from the form, and
						// the envelope check rejects a form the user can't have.
						bitmap |= admSym.Bitmap
					} else {
						glog.Warningf("%s LLM word problem missing symbolic_expression; scoring from prose (under-rated): %q", logPrefix, adm.Expr)
					}
				}

				// Enforce structural invariants before the envelope check, so
				// a multi-step problem the validator under-reported is both
				// stamped correctly AND correctly rejected for a user who
				// can't have it (#246).
				bitmap = mathcore.NormalizeProblemBitmap(bitmap)

				// Envelope: every stamped bit must be enabled for this user.
				if v := mathcore.EnvelopeViolation(bitmap, settings.ProblemTypeBitmap); v != "" {
					funnel.reject(rejectEnvelope)
					glog.Infof("%s LLM envelope reject [%s]: %q", logPrefix, v, adm.Expr)
					continue
				}

				// Convert to an api.Problem. The (possibly rewritten)
				// canonical expression is what gets stored and hashed.
				model = &Problem{}
				model.Generator = llm_generator.VERSION
				model.ProblemTypeBitmap = bitmap
				model.Expression = adm.Expr
				model.SymbolicExpression = symbolicExpr
				model.Answer = p.Answer
				// Keep the explanation consistent with a stage-1.5 rewrite:
				// the kid must not see the letter the expression no longer has.
				model.Explanation = RewriteLetterInProse(p.Explanation, adm.RewroteLetter)
				// Computed difficulty only; LLM self-report is debug logging. A
				// word problem is scored from its symbolic_expression.
				model.Difficulty = mathcore.ComputeProblemDifficulty(adm.Expr, symbolicExpr)
				model.DifficultyVersion = mathcore.DifficultyVersion
				glog.Infof("%s LLM problem: %s computed_diff=%g bitmap=%d (LLM raw=%g)", logPrefix, model.Expression, model.Difficulty, model.ProblemTypeBitmap, p.Difficulty)

				// Use expression hash as model.Id
				h := fnv.New32a()
				h.Write([]byte(model.Expression))
				model.Id = h.Sum32()

				// Check for collisions
				_, status, _, err := a.problemManager.Get(model.Id)
				// There is certainly no collision iff we receive a 404
				if status != http.StatusNotFound {
					funnel.reject(rejectCollision)
					model = nil
					continue
				}
				uniqueIds[model.Id] = true

				// Write to database
				status, msg, err := a.problemManager.Create(model)
				if err != nil {
					funnel.reject(rejectCreate)
					glog.Errorf("%s could not create LLM problem (%d: %s): %v", logPrefix, status, msg, err)
					model = nil
					continue
				}
				funnel.inserted++
				newCount += 1
				newProblem = model
			}
			glog.Infof("%s LLM %s", logPrefix, funnel)
		}
	}

	glog.Infof("%s generator numProblems requested: %d vs unique problems generated: %d and new problems generated: %d", logPrefix, numProblems, len(uniqueIds), newCount)

	// Just return the last problem added
	if newProblem == nil {
		return nil, errors.New("Failed to produce any valid new problem.")
	}
	return newProblem, nil
}
