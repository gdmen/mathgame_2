// Package api contains api routes, handlers, and models
package api // import "garydmenezes.com/mathgame/server/api"

import (
	"errors"
	"fmt"
	"hash/fnv"
	"math/rand"
	"net/http"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/golang/glog"

	heuristic_generator "garydmenezes.com/mathgame/server/generator"
	llm_generator "garydmenezes.com/mathgame/server/llm_generator"
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

	// defaultGradeLevel is used when settings.GradeLevel is 0 (legacy users
	// or users who somehow skipped the grade-selection step in onboarding).
	// Stops us from ever writing a grade_level=0 problem which the selection
	// filter (grade_level > 0) would then permanently exclude.
	defaultGradeLevel = 3
)

// effectiveGradeLevel returns the grade we should tag a newly-generated
// problem with. Never zero: legacy users still on grade_level=0 get a
// sensible fallback derived from their target_difficulty, or a hard
// default if that isn't useful either.
//
// Derivation: the adaptive cap is grade*2+4, so invert: grade = (target-4)/2.
// target=6  -> grade 1
// target=10 -> grade 3
// target=14 -> grade 5
// target=20 -> grade 8
func effectiveGradeLevel(settings *Settings) int {
	if settings.GradeLevel > 0 {
		return settings.GradeLevel
	}
	if settings.TargetDifficulty >= 6 {
		g := int((settings.TargetDifficulty - 4) / 2)
		if g < 1 {
			g = 1
		}
		if g > 8 {
			g = 8
		}
		return g
	}
	return defaultGradeLevel
}

// Get all problem ids that satisfy this ProblemTypeBitmap and have similar Difficulty
func (a *Api) getSatisfyingProblemIds(logPrefix string, c *gin.Context, settings *Settings, prevIds *[]uint32) (*[]uint32, error) {
	permutations := GetProblemTypePermutations(ProblemType(settings.ProblemTypeBitmap))
	// Special case to guarantee word problems if they're turned on
	if (ProblemType(settings.ProblemTypeBitmap) & WORD) != 0 {
		res := []ProblemType{}
		for _, pt := range permutations {
			if (pt & WORD) != 0 {
				res = append(res, pt)
			}
		}
		permutations = res
	}
	if len(permutations) == 0 {
		return &([]uint32{}), nil
	}

	diffLowerBound := settings.TargetDifficulty - problemSelectionEpsilon
	diffUpperBound := settings.TargetDifficulty + problemSelectionEpsilon
	// Universal difficulty allows cross-grade pool sharing: a grade 5 user
	// whose target drifts to 5 can draw from problems generated for other
	// grades as long as the computed difficulty matches. BUT grade_level=0
	// is the sentinel for backfilled/ungraded legacy rows that should never
	// be served; keep the always-exclude on that.
	sql := fmt.Sprintf("problem_type_bitmap IN (%s) AND difficulty >= %g and difficulty <= %g AND disabled=0 AND grade_level > 0;",
		strings.Replace(strings.Trim(fmt.Sprint(permutations), "[]"), " ", ",", -1),
		diffLowerBound,
		diffUpperBound,
	)
	if len(*prevIds) > 0 {
		idFilter := fmt.Sprintf("id NOT IN (%s) AND ", strings.Replace(strings.Trim(fmt.Sprint(*prevIds), "[]"), " ", ",", -1))
		sql = idFilter + sql
	}
	glog.Infof("getSatisfyingProblemIds sql: select * from problems where %s\n", sql)
	problemIds, status, msg, err := a.problemManager.CustomIdList(sql)
	if HandleMngrResp(logPrefix, c, status, msg, err, problemIds) != nil {
		return nil, err
	}
	return problemIds, nil
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

	// Try topic-weighted selection first
	topicStats, tsErr := a.getTopicStats(settings.UserId)
	if tsErr != nil {
		glog.Errorf("%s getTopicStats: %v (falling back to default selection)", logPrefix, tsErr)
		topicStats = nil
	}

	if topicStats != nil && len(topicStats) > 0 {
		targetTopic, topicDiff := chooseWeightedTopic(topicStats, settings.ProblemTypeBitmap, settings.TargetDifficulty, rand.Intn)
		if targetTopic != 0 {
			glog.Infof("%s topic-weighted selection: topic=%d difficulty=%.2f", logPrefix, targetTopic, topicDiff)
			// Build a topic-specific settings to query with the topic's difficulty
			topicSettings := *settings
			topicSettings.TargetDifficulty = topicDiff
			// Only include permutations that contain the target topic
			topicSettings.ProblemTypeBitmap = settings.ProblemTypeBitmap // keep all enabled, filter below
			pids, err := a.getSatisfyingProblemIdsForTopic(logPrefix, c, &topicSettings, prevIds, targetTopic)
			if err == nil && len(*pids) > 0 {
				pid := a.pickWithRecencyBias(logPrefix, settings.UserId, *pids)
				p, status, msg, err := a.problemManager.Get(pid)
				if err == nil && status == http.StatusOK {
					return p, nil
				}
				glog.Infof("%s topic-weighted fetch failed (id=%d): %s : %v", logPrefix, pid, msg, err)
			}
			// Topic-specific pool too small, trigger background generation
			if err == nil && len(*pids) < minSelectionPool {
				glog.Infof("%s topic pool small (%d), generating more", logPrefix, len(*pids))
				a.generateProblemsBackground(logPrefix, &topicSettings)
			}
		}
	}

	// Fall back to default (non-topic-weighted) selection
	pids, err := a.getSatisfyingProblemIds(logPrefix, c, settings, prevIds)
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
	inputProblemType := ProblemType(settings.ProblemTypeBitmap)
	heuristicType := inputProblemType &^ WORD
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

// getSatisfyingProblemIdsForTopic is like getSatisfyingProblemIds but only returns
// problems whose bitmap contains the target topic.
func (a *Api) getSatisfyingProblemIdsForTopic(logPrefix string, c *gin.Context, settings *Settings, prevIds *[]uint32, targetTopic uint64) (*[]uint32, error) {
	permutations := GetProblemTypePermutations(ProblemType(settings.ProblemTypeBitmap))
	// Filter to permutations containing the target topic
	filtered := []ProblemType{}
	for _, pt := range permutations {
		if (uint64(pt) & targetTopic) != 0 {
			filtered = append(filtered, pt)
		}
	}
	if len(filtered) == 0 {
		empty := []uint32{}
		return &empty, nil
	}

	diffLowerBound := settings.TargetDifficulty - problemSelectionEpsilon
	diffUpperBound := settings.TargetDifficulty + problemSelectionEpsilon
	// See note in getSatisfyingProblemIds: cross-grade sharing is OK, but
	// grade_level=0 is always excluded as the legacy-row sentinel.
	sql := fmt.Sprintf("problem_type_bitmap IN (%s) AND difficulty >= %g and difficulty <= %g AND disabled=0 AND grade_level > 0;",
		strings.Replace(strings.Trim(fmt.Sprint(filtered), "[]"), " ", ",", -1),
		diffLowerBound,
		diffUpperBound,
	)
	if len(*prevIds) > 0 {
		idFilter := fmt.Sprintf("id NOT IN (%s) AND ", strings.Replace(strings.Trim(fmt.Sprint(*prevIds), "[]"), " ", ",", -1))
		sql = idFilter + sql
	}
	glog.Infof("getSatisfyingProblemIdsForTopic sql: select id from problems where %s\n", sql)
	problemIds, status, msg, err := a.problemManager.CustomIdList(sql)
	if HandleMngrResp(logPrefix, c, status, msg, err, problemIds) != nil {
		return nil, err
	}
	return problemIds, nil
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

// runHeuristicGenerator generates problems using the heuristic generator.
// Supports ADDITION, SUBTRACTION, MULTIPLICATION, DIVISION, FRACTIONS, NEGATIVES.
// WORD problems should be generated via the LLM generator instead.
// Returns the last new problem created, the count of new problems, and the set
// of unique IDs.
//
// Does NOT take a gin.Context: this function may run in a background goroutine
// after the originating request has returned. Writing to a stale/reused context
// from a background path corrupts unrelated in-flight requests. Errors are
// logged via glog; callers decide how to handle a nil return.
func (a *Api) runHeuristicGenerator(logPrefix string, settings *Settings, numProblems int, problemType ProblemType) (*Problem, int, map[uint32]bool) {
	uniqueIds := map[uint32]bool{}
	newCount := 0
	var newProblem *Problem
	operations := []string{}
	if (ADDITION & problemType) > 0 {
		operations = append(operations, "+")
	}
	if (SUBTRACTION & problemType) > 0 {
		operations = append(operations, "-")
	}
	if (MULTIPLICATION & problemType) > 0 {
		operations = append(operations, "*")
	}
	if (DIVISION & problemType) > 0 {
		operations = append(operations, "/")
	}
	if len(operations) == 0 {
		return nil, 0, uniqueIds
	}
	// Never write a grade_level=0 row. Legacy users whose settings still
	// have grade_level=0 get a derived grade from their target_difficulty
	// so their problems are selectable going forward.
	effectiveGrade := effectiveGradeLevel(settings)
	generatorOpts := &heuristic_generator.Options{
		Operations:       operations,
		Fractions:        (FRACTIONS & problemType) > 0,
		Negatives:        (NEGATIVES & problemType) > 0,
		TargetDifficulty: settings.TargetDifficulty,
		GradeLevel:       effectiveGrade,
	}
	for i := 0; i < numProblems; i++ {
		model := &Problem{}
		model.Generator = heuristic_generator.VERSION
		var err error
		var heuristicDiff float64
		model.Expression, model.Answer, heuristicDiff, err = heuristic_generator.GenerateProblem(generatorOpts)
		if err != nil {
			if _, ok := err.(*heuristic_generator.OptionsError); ok {
				glog.Errorf("%s Failed options validation: %v", logPrefix, err)
				return nil, newCount, uniqueIds
			}
			glog.Errorf("%s Couldn't generate problem: %v", logPrefix, err)
			continue
		}
		// Compute universal difficulty from the generated expression.
		// Stored difficulty is a function of the problem itself, not the
		// requester's target. This lets problems be shared across grades
		// and users with different targets.
		model.Difficulty = ComputeProblemDifficulty(model.Expression)
		model.GradeLevel = effectiveGrade
		glog.Infof("%s heuristic problem: %s = %s (grade=%d computed_diff=%g raw=%g)", logPrefix, model.Expression, model.Answer, model.GradeLevel, model.Difficulty, heuristicDiff)
		h := fnv.New32a()
		h.Write([]byte(model.Expression))
		model.Id = h.Sum32()
		_, status, _, err := a.problemManager.Get(model.Id)
		if status != http.StatusNotFound {
			continue
		}
		uniqueIds[model.Id] = true
		model.ProblemTypeBitmap = detectProblemTypeBitmap(model.Expression)
		status, msg, err := a.problemManager.Create(model)
		if err != nil {
			glog.Errorf("%s could not create heuristic problem (%d: %s): %v", logPrefix, status, msg, err)
			continue
		}
		newCount++
		newProblem = model
	}
	return newProblem, newCount, uniqueIds
}

// detectProblemTypeBitmap inspects a generated expression and returns the
// bitmap of problem types it contains. Used to tag heuristic-generated
// problems so they're selected correctly by type filter.
//
// Heuristic generator formatting conventions:
//   - Binary operators are surrounded by spaces: "42 / 6", "3 + 5"
//   - Fractions have no surrounding spaces on their slash: "1/2 + 3/4"
//   - Negative numbers appear as "-N" or with a leading minus in expressions.
func detectProblemTypeBitmap(expr string) uint64 {
	var bitmap uint64 = 0
	// Subtraction has a space-dash-space pattern; a bare leading "-" is a
	// negative number, not a subtraction operator.
	if strings.Contains(expr, " - ") {
		bitmap |= uint64(SUBTRACTION)
	}
	if strings.Contains(expr, " + ") {
		bitmap |= uint64(ADDITION)
	}
	if strings.Contains(expr, " * ") {
		bitmap |= uint64(MULTIPLICATION)
	}
	if strings.Contains(expr, " / ") {
		bitmap |= uint64(DIVISION)
	}
	// Fractions: a "/" without surrounding spaces (e.g., "1/2").
	// Easy test: any "/" that isn't part of " / ".
	if strings.Count(expr, "/") > strings.Count(expr, " / ") {
		bitmap |= uint64(FRACTIONS)
	}
	// Negatives: presence of a unary minus (leading "-" or "-" after operator/space).
	// Cheap heuristic: "-" at start, or " -" appearing where no subtraction operator follows.
	if strings.HasPrefix(expr, "-") {
		bitmap |= uint64(NEGATIVES)
	}
	return bitmap
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
	inputProblemType := ProblemType(settings.ProblemTypeBitmap)
	// Try the LLM generator first. It produces richer content (word problems,
	// varied phrasings, curriculum-aligned context) and should be the primary
	// source for every problem type it can handle. The heuristic generator is
	// a deterministic, offline fallback for when OpenAI is unreachable or
	// returns no valid problems.
	{
		// Never tag a new problem with grade_level=0. Legacy users still on
		// grade 0 get a derived grade so their problems are selectable.
		effectiveGrade := effectiveGradeLevel(settings)
		generatorOpts := &llm_generator.Options{
			Features:         ProblemTypeToFeatures(inputProblemType),
			TargetDifficulty: settings.TargetDifficulty,
			NumProblems:      numProblems, // we still return just one problem, but this lets us reduce the number of OpenAI calls we need to make
			GradeLevel:       effectiveGrade,
		}

		var err error
		var generatorProblems []llm_generator.Problem
		generatorProblems, err = llm_generator.GenerateProblem(generatorOpts)
		if err != nil {
			// Fall back to heuristic when OpenAI fails. Strip WORD since the
			// heuristic doesn't produce word problems, and fall back on the
			// remaining arithmetic types.
			heuristicType := inputProblemType &^ WORD
			if heuristicType != 0 {
				glog.Infof("%s OpenAI failed (%v), falling back to heuristic generator", logPrefix, err)
				newProblem, newCount, uniqueIds = a.runHeuristicGenerator(logPrefix, settings, numProblems, heuristicType)
			} else {
				msg := "Couldn't generate problems"
				glog.Errorf("%s %s: %v", logPrefix, msg, err)
				return nil, err
			}
		} else {
			for _, p := range generatorProblems {
				glog.Infof("%s generated problem: %v", logPrefix, p)

				// Convert to an api.Problem
				model = &Problem{}
				model.Generator = llm_generator.VERSION
				model.ProblemTypeBitmap = uint64(FeaturesToProblemType(p.Features))
				model.Expression = strings.TrimSpace(p.Expression)
				model.Answer = p.Answer
				model.Explanation = p.Explanation
				// Compute universal difficulty from the expression itself.
				// LLM's self-reported difficulty is logged for debugging only.
				model.Difficulty = ComputeProblemDifficulty(model.Expression)
				model.GradeLevel = effectiveGrade
				glog.Infof("%s LLM problem: %s computed_diff=%g grade=%d (LLM raw=%g)", logPrefix, model.Expression, model.Difficulty, model.GradeLevel, p.Difficulty)

				// Use expression hash as model.Id
				h := fnv.New32a()
				h.Write([]byte(model.Expression))
				model.Id = h.Sum32()

				// Check for collisions
				_, status, _, err := a.problemManager.Get(model.Id)
				// There is certainly no collision iff we receive a 404
				if status != http.StatusNotFound {
					//glog.Infof("%s could not verify uniqueness of problem: (%d: %v)", logPrefix, status, err)
					model = nil
					continue
				}
				uniqueIds[model.Id] = true

				// Validate problem (includes grade alignment check if grade is set)
				err = llm_generator.ValidateProblemWithGrade(&p, effectiveGrade)
				if err != nil {
					glog.Infof("%s problem validation failed: %v", logPrefix, err)
					model = nil
					continue
				}

				// Difficulty calibration: reject if LLM's self-reported difficulty
				// diverges too far from what we requested (> 100% off)
				if settings.TargetDifficulty > 0 && p.Difficulty > 0 {
					ratio := p.Difficulty / settings.TargetDifficulty
					if ratio < 0.5 || ratio > 2.0 {
						glog.Infof("%s difficulty calibration reject: requested=%.1f, LLM reported=%.1f (ratio=%.2f)",
							logPrefix, settings.TargetDifficulty, p.Difficulty, ratio)
						model = nil
						continue
					}
				}

				// Write to database
				status, msg, err := a.problemManager.Create(model)
				if err != nil {
					glog.Errorf("%s could not create LLM problem (%d: %s): %v", logPrefix, status, msg, err)
					model = nil
					continue
				}
				newCount += 1
				newProblem = model
			}
		}
	}

	glog.Infof("%s generator numProblems requested: %d vs unique problems generated: %d and new problems generated: %d", logPrefix, numProblems, len(uniqueIds), newCount)

	// Just return the last problem added
	if newProblem == nil {
		return nil, errors.New("Failed to produce any valid new problem.")
	}
	return newProblem, nil
}
