package generator

// heuristic2.go: the heuristic_2.0 builder — a compositional, answer-first,
// difficulty-targeting generator on the mathcore render-only AST.
//
// The shape of the design:
//   - ANSWER-FIRST, COMPOSITIONAL construction. There is ONE recursion, expand:
//     it grows the AST outward from a chosen value, and every node's value is
//     known as it is built. Operators are node choices; concepts are operand
//     realizations chosen inside the value split (a fraction/decimal/percent/
//     negative operand) or at a leaf. Concepts COMPOSE because the recursion
//     composes — a single problem can carry a fraction here, a division there,
//     a decimal there. There is deliberately no per-concept template dispatch.
//   - Per-node invariants keep every candidate clean by construction: division
//     splits are exact (a = v*b, or an integer dividend over a proper fraction,
//     a = v*p/q), integers stay integers unless a fraction/decimal/percent
//     concept is active, and values stay non-negative unless NEGATIVES is
//     enabled. FRACTION OPERANDS STAY PROPER AND SMALL (SMALL-bracket
//     components): MEDIUM/LARGE magnitude rides on an integer operand
//     (24 ÷ 2/3 = 36), never on an inflated numerator.
//   - KNOB INVERSION sizes the build: RawForDifficulty(target) gives a raw
//     target; planConfig picks, minimal-concept-first, how much magnitude /
//     chain / concept to spend (binding to the shared mathcore difficulty
//     constants, never a private copy).
//   - The canonical pipeline is the VERIFIER, not the source of truth for the
//     answer: every candidate is rendered and run through AdmitExpression +
//     VerifyAnswerSymbolic + DetectProblemTypeBitmap + EnvelopeViolation +
//     ComputeProblemDifficulty. Generate-and-select keeps the closest in-window
//     survivor. Everything FAILS CLOSED — a construction slip costs a retry,
//     never a wrong or out-of-envelope problem; a near-ceiling cell degrades to
//     the closest valid problem, then a deterministic fallback, never a spin.
//   - PEMDAS is emergent (a multiplicative subtree read after an additive
//     operator), so it is not constructed specially: the canonical detector on
//     the rendered candidate decides whether it fired, and the envelope check
//     rejects a candidate that fires PEMDAS when the bit is disabled.

import (
	"math"
	"math/big"
	"math/rand"

	"garydmenezes.com/mathgame/server/mathcore"
)

// VERSION is the generator version string stamped on created problems.
// See docs/generator-versions.md for version history.
const VERSION = "heuristic_2.0"

// OptionsError is returned when the envelope cannot produce a problem.
type OptionsError struct{ s string }

func (e *OptionsError) Error() string { return e.s }

const (
	targetWindow  = 1.5 // selection epsilon; an in-window hit serves at the target
	buildAttempts = 220 // generate-and-select budget per call
)

// buildCtx is the per-attempt plan: the envelope, the magnitude bracket, the
// concept subset this attempt will try to use, and the structural budget. It is
// produced by planConfig (the knob inverter).
type buildCtx struct {
	bitmap     mathcore.ProblemType
	maxOperand int // magnitude bracket cap — the hard envelope ceiling
	operandCap int // aimed operand size for this attempt (<= maxOperand)
	depth      int // arithmetic-tree growth budget (~ operator count)
	rng        *rand.Rand
	rawTarget  float64

	// concept flavors this attempt may apply (each a subset of bitmap)
	fractions  bool
	mismatched bool
	decimals   bool
	percent    bool
	negatives  bool
	pemdas     bool
	// equation form (mutually exclusive, the unknown rule)
	variable bool
	missing  bool
}

// BuildProblem (heuristic_2.0) builds a symbolic problem for the envelope bitmap
// aimed at target difficulty. Returns (expression, answer, error), where
// expression is the canonical grammar form (unspaced a/b fractions). The caller
// stores that as symbolic_expression and derives the \frac display for
// expression (mathcore.DisplayExpression); the heuristic emits no WORD problems.
func BuildProblem(bitmap mathcore.ProblemType, target float64, rng *rand.Rand) (string, string, error) {
	if coreOpsMask(bitmap) == 0 {
		return "", "", &OptionsError{s: "no core operation enabled in bitmap"}
	}
	rawTarget := mathcore.RawForDifficulty(target)

	var bestExpr, bestAns string
	bestErr := math.MaxFloat64

	for attempt := 0; attempt < buildAttempts; attempt++ {
		ctx := planConfig(bitmap, rawTarget, rng)
		node, unknown, ok := buildOne(ctx)
		if !ok {
			continue
		}
		expr := mathcore.Render(node)
		adm := mathcore.AdmitExpression(expr)
		if adm.RejectStage != "" {
			continue
		}
		// Answer: the unknown's value for an equation, else the value of the
		// constructed tree. Render is faithful, so this equals the rendered
		// expression's value; VerifyAnswerSymbolic re-checks it against the
		// rendered tokens below, so a construction slip is still caught.
		var ansVal *big.Rat
		if unknown != nil {
			ansVal = unknown
		} else {
			v, err := mathcore.Eval(node, nil)
			if err != nil {
				continue
			}
			ansVal = v
		}
		if bitmap&mathcore.NEGATIVES == 0 && ansVal.Sign() < 0 {
			continue // negative answer leaks negatives into a no-negatives envelope
		}
		ans := formatAnswer(ansVal, ctx.decimals)
		if mathcore.VerifyAnswerSymbolic(adm.Tokens, ans) != nil {
			continue
		}
		bm := mathcore.NormalizeProblemBitmap(adm.Bitmap)
		// Reject trivial candidates: a real problem must carry at least one
		// operation or an unknown to solve for. A bare number ("7") or "1 = 1"
		// detects no such bit — it passes the envelope (0 is a subset of
		// everything) but is useless and would only collide.
		if coreOpsMask(mathcore.ProblemType(bm)) == 0 &&
			bm&uint64(mathcore.MISSING_NUMBER|mathcore.SINGLE_VARIABLE) == 0 {
			continue
		}
		if mathcore.EnvelopeViolation(bm, uint64(bitmap)) != "" {
			continue
		}
		d := mathcore.ComputeProblemDifficulty(adm.Expr, "")
		if e := math.Abs(d - target); e < bestErr {
			bestErr, bestExpr, bestAns = e, adm.Expr, ans
			if e <= targetWindow {
				return adm.Expr, ans, nil
			}
		}
	}
	if bestExpr != "" {
		return bestExpr, bestAns, nil // closest achievable (near-ceiling / coarse cell)
	}
	return fallback(bitmap, rng)
}

// ---- the knob inverter (planConfig) ----

// planConfig sizes one build attempt toward rawTarget under the minimal-concept
// policy: magnitude and chain length are the near-continuous dials; concepts are
// coarse jumps added cheapest-first only as the target outgrows what magnitude
// and chain can supply. The randomized magnitude assumption spreads attempts
// across the magnitude/concept tradeoff so generate-and-select can keep the
// closest. Binds to the shared mathcore constants — no private copies.
func planConfig(bitmap mathcore.ProblemType, rawTarget float64, rng *rand.Rand) buildCtx {
	ctx := buildCtx{bitmap: bitmap, rng: rng, rawTarget: rawTarget, maxOperand: bracketCap(bitmap)}

	// Op weight available in this envelope (the hardest core op).
	opW := 1.0
	if bitmap&mathcore.SUBTRACTION != 0 {
		opW = math.Max(opW, mathcore.WeightSub)
	}
	if bitmap&mathcore.MULTIPLICATION != 0 {
		opW = math.Max(opW, mathcore.WeightMul)
	}
	if bitmap&mathcore.DIVISION != 0 {
		opW = math.Max(opW, mathcore.WeightDiv)
	}

	// Structure budget: a chain length (operator count) up to the ceiling's
	// MaxChainLen when CHAINED is enabled, biased shorter for magnitude-capable
	// envelopes so big operands carry difficulty rather than long chains.
	maxChain := 1
	if bitmap&mathcore.CHAINED_OPERATIONS != 0 {
		maxChain = mathcore.MaxChainLen
	}
	depth := 1 + rng.Intn(maxChain)
	if bitmap&(mathcore.MEDIUM_NUMBERS|mathcore.LARGE_NUMBERS) != 0 && rng.Intn(2) == 0 {
		depth = 1 + rng.Intn(min(2, maxChain))
	}
	ctx.depth = depth
	structure := 1.0 + mathcore.StructurePerExtraOp*float64(depth-1)

	// Equation form is OPTIONAL (an unknown is a MAY, not a MUST): only some
	// attempts introduce one, so an envelope that allows SINGLE_VARIABLE/MISSING
	// can still build easy plain problems for low targets. At most one unknown per
	// problem (SINGLE_VARIABLE xor MISSING — the unknown rule). SINGLE_VARIABLE is
	// a coarse x5 jump, so the magnitude solve below accounts for it.
	hasSV := bitmap&mathcore.SINGLE_VARIABLE != 0
	hasMissing := bitmap&mathcore.MISSING_NUMBER != 0
	if (hasSV || hasMissing) && rng.Intn(2) == 0 {
		if hasSV && (!hasMissing || rng.Intn(2) == 0) {
			ctx.variable = true // x5 concept folded into the magnitude solve via concForVariable
		} else {
			ctx.missing = true
			structure += mathcore.StructureMissing
		}
	}

	// Concept budget: how much multiplier we still need beyond op*structure at
	// the magnitude ceiling. Add cheapest enabled concepts (log-closest) until
	// covered — the minimal-concept policy.
	const magMin = 0.6
	magCeil := math.Log10(float64(ctx.maxOperand)+1) + 0.3
	assumedMag := magMin + rng.Float64()*(magCeil-magMin)
	concForVariable := 1.0
	if ctx.variable {
		concForVariable = mathcore.ConceptVariable
	}
	need := rawTarget / (opW * structure * assumedMag * concForVariable)
	ctx.chooseConcepts(need)

	// Solve the TARGET operand magnitude for the residual after op/structure/
	// concept, so construction sizes operands to the target rather than maxing
	// the bracket (which would overshoot an easy target in a large-number
	// envelope). The bracket cap (maxOperand) stays the hard envelope ceiling;
	// operandCap is the aimed size, with multiplicative noise for spread.
	conceptF := ctx.conceptFactor()
	if ctx.variable {
		conceptF *= mathcore.ConceptVariable
	}
	magNeeded := rawTarget / (opW * structure * conceptF)
	cap := math.Pow(10, magNeeded-0.3) - 1
	cap *= math.Exp(rng.NormFloat64() * 0.2)
	ctx.operandCap = clampOperandCap(int(math.Round(cap)), bitmap, ctx.maxOperand, rng)
	return ctx
}

// conceptFactor is the difficulty multiplier of the value-concepts chosen for
// this attempt (matches the formula's concept product so the magnitude solve
// aims true).
func (ctx buildCtx) conceptFactor() float64 {
	f := 1.0
	if ctx.fractions {
		f *= mathcore.ConceptFractions
		if ctx.mismatched {
			f *= mathcore.ConceptMismatched
		}
	}
	if ctx.negatives {
		f *= mathcore.ConceptNegatives
	}
	if ctx.pemdas {
		f *= mathcore.ConceptPEMDAS
	}
	if ctx.decimals {
		f *= mathcore.ConceptDecimals
	}
	if ctx.percent {
		f *= mathcore.ConceptPercent
	}
	return f
}

// clampOperandCap keeps the aimed operand size within an envelope-safe magnitude
// bracket: <=12 stamps no magnitude bit; (12,99] needs MEDIUM; (99,9999] needs
// LARGE. A solved cap landing in a disabled bracket is pulled to the nearest
// enabled one.
func clampOperandCap(cap int, bitmap mathcore.ProblemType, hardCap int, rng *rand.Rand) int {
	if cap < 2 {
		cap = 2 + rng.Intn(6)
	}
	if cap > hardCap {
		cap = hardCap
	}
	if cap > mathcore.SmallMaxOperand && bitmap&(mathcore.MEDIUM_NUMBERS|mathcore.LARGE_NUMBERS) == 0 {
		cap = mathcore.SmallMaxOperand
	}
	if cap > mathcore.MediumMaxOperand && bitmap&mathcore.LARGE_NUMBERS == 0 {
		cap = mathcore.MediumMaxOperand
	}
	return cap
}

// chooseConcepts enables a minimal subset of the envelope's value-concept bits
// (fractions/decimals/percent/negatives) to cover need, cheapest-first by
// multiplier, stopping when the next concept would overshoot need more than the
// current shortfall (so the difficulty window absorbs the residual rather than a
// coarse jump blowing past it). Shuffled so equal-cost concepts get fair turns.
func (ctx *buildCtx) chooseConcepts(need float64) {
	b := ctx.bitmap
	type opt struct {
		on   func()
		mult float64
		ok   bool
	}
	opts := []opt{
		{func() { ctx.negatives = true }, mathcore.ConceptNegatives, b&mathcore.NEGATIVES != 0},
		{func() { ctx.pemdas = true }, mathcore.ConceptPEMDAS,
			b&mathcore.PEMDAS != 0 && b&mathcore.CHAINED_OPERATIONS != 0 &&
				b&(mathcore.MULTIPLICATION|mathcore.DIVISION|mathcore.SUBTRACTION) != 0},
		{func() { ctx.decimals = true }, mathcore.ConceptDecimals, b&mathcore.DECIMALS != 0},
		{func() { ctx.percent = true }, mathcore.ConceptPercent, b&mathcore.PERCENTAGES != 0},
		{func() { ctx.fractions = true }, mathcore.ConceptFractions, b&mathcore.FRACTIONS != 0},
	}
	ctx.rng.Shuffle(len(opts), func(i, j int) { opts[i], opts[j] = opts[j], opts[i] })
	got := 1.0
	logDist := func(v float64) float64 { return math.Abs(math.Log(need) - math.Log(v)) }
	for {
		best, bestImpr := -1, 0.0
		for i, o := range opts {
			if !o.ok {
				continue
			}
			if impr := logDist(got) - logDist(got*o.mult); impr > bestImpr {
				best, bestImpr = i, impr
			}
		}
		if best < 0 {
			break
		}
		opts[best].on()
		got *= opts[best].mult
		opts[best].ok = false
	}
	// MISMATCHED stacks on FRACTIONS when enabled and it helps close the gap.
	if ctx.fractions && b&mathcore.MISMATCHED_DENOMINATORS != 0 &&
		logDist(got*mathcore.ConceptMismatched) < logDist(got) {
		ctx.mismatched = true
	}
}

// ---- construction ----

// buildOne realizes one candidate for the plan. The second return is the
// unknown's value for an EQUATION (variable / missing); nil for a plain
// expression (the caller takes the answer by evaluating the render). ok=false
// means this attempt could not construct cleanly (the caller retries).
func buildOne(ctx buildCtx) (mathcore.Node, *big.Rat, bool) {
	switch {
	case ctx.variable:
		return buildVariableEquation(ctx)
	case ctx.missing:
		return buildMissingEquation(ctx)
	default:
		ans := chooseAnswer(ctx)
		node, ok := expand(ans, ctx.depth, ctx)
		if !ok {
			return nil, nil, false
		}
		return node, nil, true
	}
}

// chooseAnswer picks a concept-friendly target value to grow the tree from: a
// fraction/decimal when those concepts are active (so the tree's operands fall
// out as fractions/decimals naturally), else an integer sized to the bracket.
func chooseAnswer(ctx buildCtx) *big.Rat {
	cap := ctx.operandCap
	switch {
	case ctx.fractions:
		// With mul/div available, half the time seed an INTEGER answer so a proper
		// fraction rides on the tree as an operand whose integer partner carries the
		// magnitude (24 ÷ 2/3 = 36) — this is how a fraction problem reaches
		// MEDIUM/LARGE without inflating a numerator (splitDiv/MulIntWithFrac).
		// Otherwise a fraction answer; its components stay small (withinMag caps
		// the multiplicative splits), and additive splits keep it bounded anyway.
		if ctx.bitmap&(mathcore.MULTIPLICATION|mathcore.DIVISION) != 0 && ctx.rng.Intn(2) == 0 {
			return big.NewRat(int64(randRange(2, max(2, cap*(ctx.depth+1)), ctx.rng)), 1)
		}
		d := pickDenom(ctx)
		n := 1 + ctx.rng.Intn(d*3)
		return big.NewRat(int64(n), int64(d))
	case ctx.decimals:
		scale := int64(decimalScale(ctx))
		return big.NewRat(int64(1+ctx.rng.Intn(cap*int(scale)/10+1)), scale)
	default:
		// Scale the seed with the tree size so an additive split yields operands
		// near operandCap (not a tiny undershoot), and span a wide range so
		// attempts vary and generate-and-select can match different op shapes.
		return big.NewRat(int64(randRange(2, max(2, cap*(ctx.depth+1)), ctx.rng)), 1)
	}
}

// randRange returns a uniform int in [lo, hi] (inclusive). hi<lo collapses to lo.
func randRange(lo, hi int, rng *rand.Rand) int {
	if hi <= lo {
		return lo
	}
	return lo + rng.Intn(hi-lo+1)
}

// expand grows a subtree evaluating exactly to v. With budget left it splits v
// into a OP b (answer-first, exact) and recurses; otherwise it realizes v as a
// leaf. This single recursion is where concepts compose: each split independently
// chooses an enabled op and operand flavor.
func expand(v *big.Rat, depth int, ctx buildCtx) (mathcore.Node, bool) {
	if depth <= 0 || (depth < ctx.depth && ctx.rng.Intn(3) == 0) {
		return realizeLeaf(v, ctx)
	}
	op, a, b, ok := splitValue(v, depth, ctx)
	if !ok {
		return realizeLeaf(v, ctx)
	}
	left, lok := expand(a, depth-1, ctx)
	right, rok := expand(b, depth-1, ctx)
	if !lok || !rok {
		return realizeLeaf(v, ctx)
	}
	// PEMDAS by construction: parenthesizing the right operand of '-' or '/'
	// flips the inner operands' effective order vs a naive left-to-right read
	// ("a - (b - c)" != "a - b - c"), so the canonical requiresPEMDAS fires. We
	// only do this when the right side is itself a multi-op subtree (so the
	// parens are load-bearing). Emergent PEMDAS via "x + y*z" needs no help. The
	// canonical detector + envelope check remain authoritative — a paren shape
	// that doesn't actually fire just scores lower; one that fires when PEMDAS is
	// disabled is rejected.
	if ctx.pemdas && (op == '-' || op == '/') {
		if _, ok := right.(mathcore.BinaryExpr); ok {
			right = mathcore.Paren{X: right}
		}
	}
	return mathcore.BinaryExpr{Op: op, L: left, R: right}, true
}

// splitValue chooses an enabled operator and an exact answer-first split of v
// into (a, b) with a OP b == v, honoring the magnitude bracket and (when no
// fraction/decimal/percent concept is active) integer operands. Division is
// clean by construction. Returns ok=false when no clean split is available.
func splitValue(v *big.Rat, depth int, ctx buildCtx) (byte, *big.Rat, *big.Rat, bool) {
	ops := enabledOps(ctx.bitmap)
	ctx.rng.Shuffle(len(ops), func(i, j int) { ops[i], ops[j] = ops[j], ops[i] })
	for _, op := range ops {
		switch op {
		case '/':
			if a, b, ok := splitDiv(v, ctx); ok {
				return '/', a, b, true
			}
		case '*':
			if a, b, ok := splitMul(v, ctx); ok {
				return '*', a, b, true
			}
		case '+':
			if a, b, ok := splitAdd(v, ctx); ok {
				return '+', a, b, true
			}
		case '-':
			if a, b, ok := splitSub(v, ctx); ok {
				return '-', a, b, true
			}
		}
	}
	return 0, nil, nil, false
}

// addendStyle picks how an additive operand is flavored this split: a fraction,
// a decimal, a percent, or a plain integer — drawn from the active concepts so
// the operand carries (and composes) the concept.
func (ctx buildCtx) wantFractionalOperand() bool {
	return ctx.fractions || ctx.decimals || ctx.percent
}

// splitAdd: a + b = v. When a fractional concept is active, a is a non-integer
// operand of that flavor and b = v - a (also non-integer, rendered to match);
// otherwise integers. Keeps both operands positive.
func splitAdd(v *big.Rat, ctx buildCtx) (*big.Rat, *big.Rat, bool) {
	if ctx.fractions {
		// a = i/d, b = v - i/d. b's denominator falls out of the subtraction; it
		// may differ from d (a mismatched pair) and is still a valid fraction
		// leaf. Choose i so b stays positive.
		d := pickDenom(ctx)
		maxI := max(1, int(ratFloor(new(big.Rat).Mul(v, big.NewRat(int64(d), 1)))))
		a := big.NewRat(int64(1+ctx.rng.Intn(maxI)), int64(d))
		b := new(big.Rat).Sub(v, a)
		if b.Sign() <= 0 {
			return nil, nil, false
		}
		return a, b, true
	}
	if ctx.decimals {
		scale := int64(decimalScale(ctx))
		vs := new(big.Rat).Mul(v, big.NewRat(scale, 1))
		if !vs.IsInt() {
			return nil, nil, false
		}
		maxI := vs.Num().Int64() - 1
		if maxI < 1 {
			return nil, nil, false
		}
		i := int64(1) + int64(ctx.rng.Intn(int(maxI)))
		a := big.NewRat(i, scale)
		b := new(big.Rat).Sub(v, a)
		if b.Sign() <= 0 {
			return nil, nil, false
		}
		return a, b, true
	}
	// integer: split into two operands BOTH within operandCap, so the leaves
	// carry the aimed magnitude (not a tiny "1 + 1" undershoot) and vary.
	if !v.IsInt() {
		return nil, nil, false
	}
	n := int(v.Num().Int64())
	if n < 2 {
		return nil, nil, false
	}
	loA := max(1, n-ctx.operandCap)
	hiA := min(ctx.operandCap, n-1)
	if loA > hiA {
		return nil, nil, false // n too big to split into two <= operandCap; recurse/regrow
	}
	a := randRange(loA, hiA, ctx.rng)
	return ri(a), ri(n - a), true
}

// splitSub: a - b = v. b is a fresh positive operand, a = v + b.
func splitSub(v *big.Rat, ctx buildCtx) (*big.Rat, *big.Rat, bool) {
	if ctx.fractions {
		d := pickDenom(ctx)
		j := 1 + ctx.rng.Intn(d*2)
		b := big.NewRat(int64(j), int64(d))
		a := new(big.Rat).Add(v, b)
		if !withinMag(a, ctx) {
			return nil, nil, false
		}
		return a, b, true
	}
	if ctx.decimals {
		scale := int64(decimalScale(ctx))
		j := int64(1) + int64(ctx.rng.Intn(ctx.operandCap*int(scale)/10+1))
		b := big.NewRat(j, scale)
		a := new(big.Rat).Add(v, b)
		if !withinMag(a, ctx) {
			return nil, nil, false
		}
		return a, b, true
	}
	if !v.IsInt() {
		return nil, nil, false
	}
	b := 1 + ctx.rng.Intn(ctx.operandCap)
	a := int(v.Num().Int64()) + b
	if a > ctx.maxOperand {
		return nil, nil, false
	}
	return ri(a), ri(b), true
}

// splitMul: a * b = v. Integer factor split (v must be a positive integer with a
// factor in range), or a percent split (a = n%, b = v/(n/100)) when percent is
// the active concept.
func splitMul(v *big.Rat, ctx buildCtx) (*big.Rat, *big.Rat, bool) {
	if ctx.percent {
		// a = n%  (n in a nice set), b = v / (n/100) must be a clean integer.
		nice := []int{5, 10, 20, 25, 40, 50, 75}
		for _, tries := 0, 0; tries < 6; tries++ {
			n := nice[ctx.rng.Intn(len(nice))]
			pct := big.NewRat(int64(n), 100)
			b := new(big.Rat).Quo(v, pct)
			if b.IsInt() && b.Sign() > 0 && withinMag(b, ctx) {
				return pct, b, true // a is the percent operand
			}
		}
		return nil, nil, false
	}
	if ctx.fractions || ctx.decimals {
		if a, b, ok := splitMulFrac(v, ctx); ok {
			return a, b, true
		}
	}
	if ctx.fractions && v.IsInt() && v.Sign() > 0 {
		if a, b, ok := splitMulIntWithFrac(v, ctx); ok {
			return a, b, true
		}
	}
	if !v.IsInt() || v.Sign() <= 0 {
		return nil, nil, false
	}
	n := v.Num().Int64()
	small := int64(max(2, min(ctx.maxOperand, 12)))
	divs := divisorsInRange(n, small)
	if len(divs) == 0 {
		return nil, nil, false
	}
	a := divs[ctx.rng.Intn(len(divs))]
	return ri(int(a)), ri(int(n / a)), true
}

// splitDiv: a / b = v. Choose b in range, a = v*b (exact, so division is clean
// by construction). a must stay within the magnitude bracket.
func splitDiv(v *big.Rat, ctx buildCtx) (*big.Rat, *big.Rat, bool) {
	if v.Sign() <= 0 {
		return nil, nil, false
	}
	if ctx.fractions || ctx.decimals {
		if a, b, ok := splitDivFrac(v, ctx); ok {
			return a, b, true
		}
	}
	if ctx.fractions && v.IsInt() {
		if a, b, ok := splitDivIntByFrac(v, ctx); ok {
			return a, b, true
		}
	}
	// Integer dividend path (fraction/decimal handled above): a = v*b must be whole.
	small := max(2, min(ctx.maxOperand, 12))
	for tries := 0; tries < 6; tries++ {
		b := 2 + ctx.rng.Intn(small-1)
		a := new(big.Rat).Mul(v, big.NewRat(int64(b), 1))
		if a.IsInt() && withinMag(a, ctx) {
			return a, ri(b), true
		}
	}
	return nil, nil, false
}

// splitMulFrac splits a fractional v into a*b with at least one non-integer
// operand, so a fraction/decimal composes under multiplication. Answer-first:
// the operands are factored out of the given v so the tree stays exact;
// realizeLeaf renders each as a decimal or fraction per the active concept (the
// slash convention that keeps the product unambiguous is in
// docs/problem-generation.md).
//   - mismatched on:  two fractions with different denominators (3/8 * 5/3)
//   - mismatched off: a single non-integer operand times an integer (1/6 * 5,
//     0.2 * 3), which keeps a no-mismatched envelope satisfiable.
func splitMulFrac(v *big.Rat, ctx buildCtx) (*big.Rat, *big.Rat, bool) {
	vn, vd := v.Num().Int64(), v.Denom().Int64()
	if vd <= 1 || v.Sign() <= 0 {
		return nil, nil, false // need a fractional product to factor
	}
	small := int64(max(2, min(ctx.maxOperand, 12)))
	if ctx.mismatched {
		// a = an/vd, b = bn/d2 with an*bn = vn*d2 (so a*b = vn/vd), d2 != vd.
		for tries := 0; tries < 12; tries++ {
			d2 := int64(pickDenom(ctx))
			if d2 == vd {
				continue
			}
			divs := divisorsInRange(vn*d2, small)
			ctx.rng.Shuffle(len(divs), func(i, j int) { divs[i], divs[j] = divs[j], divs[i] })
			for _, an := range divs {
				bn := vn * d2 / an
				if bn < 1 || bn > small {
					continue
				}
				a := new(big.Rat).SetFrac64(an, vd)
				b := new(big.Rat).SetFrac64(bn, d2)
				if okFracPair(a, b, ctx) {
					return a, b, true
				}
			}
		}
		return nil, nil, false
	}
	// fraction x integer: b divides the numerator so a keeps v's denominator.
	divs := divisorsInRange(vn, small)
	ctx.rng.Shuffle(len(divs), func(i, j int) { divs[i], divs[j] = divs[j], divs[i] })
	for _, b := range divs {
		a := new(big.Rat).SetFrac64(vn/b, vd)
		if !a.IsInt() && withinMag(a, ctx) {
			return a, ri(int(b)), true
		}
	}
	return nil, nil, false
}

// splitDivFrac splits a fractional v into a/b with a fraction dividend (a = v*b).
// Any operand shape parses unambiguously (no parens needed) — see the slash
// convention in docs/problem-generation.md.
//   - mismatched on:  a, b fractions of different denominators ("5/4 / 2/3")
//   - mismatched off: a single fraction over an integer ("3/2 / 2").
func splitDivFrac(v *big.Rat, ctx buildCtx) (*big.Rat, *big.Rat, bool) {
	if v.Denom().Int64() <= 1 || v.Sign() <= 0 {
		return nil, nil, false
	}
	small := max(2, min(ctx.maxOperand, 12))
	if ctx.mismatched {
		for tries := 0; tries < 16; tries++ {
			s := pickDenom(ctx)
			b := new(big.Rat).SetFrac64(int64(1+ctx.rng.Intn(s*small)), int64(s))
			a := new(big.Rat).Mul(v, b)
			if okFracPair(a, b, ctx) {
				return a, b, true
			}
		}
		return nil, nil, false
	}
	bs := make([]int, 0, small)
	for b := 2; b <= small; b++ {
		bs = append(bs, b)
	}
	ctx.rng.Shuffle(len(bs), func(i, j int) { bs[i], bs[j] = bs[j], bs[i] })
	for _, b := range bs {
		a := new(big.Rat).Mul(v, big.NewRat(int64(b), 1))
		if !a.IsInt() && withinMag(a, ctx) {
			return a, ri(b), true
		}
	}
	return nil, nil, false
}

// splitDivIntByFrac splits an INTEGER v into "a / (p/q) = v": a is a
// medium/large integer dividend that carries the magnitude and p/q is a proper,
// small fraction (24 ÷ 2/3 = 36). a = v*p/q must be a whole number in the
// bracket. This is how a fraction-division problem reaches medium/large
// difficulty without inflating a numerator.
func splitDivIntByFrac(v *big.Rat, ctx buildCtx) (*big.Rat, *big.Rat, bool) {
	small := max(2, min(ctx.maxOperand, 12))
	vn := v.Num().Int64()
	for tries := 0; tries < 16; tries++ {
		q := int64(2 + ctx.rng.Intn(small-1))
		p := int64(1 + ctx.rng.Intn(int(q-1))) // proper: 0 < p < q
		prod := vn * p
		if prod%q != 0 {
			continue
		}
		a := prod / q // = v * p/q, the integer dividend
		if a < 2 || a > int64(ctx.maxOperand) {
			continue
		}
		return ri(int(a)), big.NewRat(p, q), true
	}
	return nil, nil, false
}

// splitMulIntWithFrac splits an INTEGER v into a proper small fraction times a
// medium/large integer that carries the magnitude (3/4 * 48 = 36, order
// randomized). b = v*q/p must be a whole number in the bracket.
func splitMulIntWithFrac(v *big.Rat, ctx buildCtx) (*big.Rat, *big.Rat, bool) {
	small := max(2, min(ctx.maxOperand, 12))
	vn := v.Num().Int64()
	for tries := 0; tries < 16; tries++ {
		q := int64(2 + ctx.rng.Intn(small-1))
		p := int64(1 + ctx.rng.Intn(int(q-1))) // proper
		prod := vn * q
		if prod%p != 0 {
			continue
		}
		b := prod / p // = v * q/p, the integer multiplicand
		if b < 2 || b > int64(ctx.maxOperand) {
			continue
		}
		frac := big.NewRat(p, q)
		if ctx.rng.Intn(2) == 0 {
			return frac, ri(int(b)), true
		}
		return ri(int(b)), frac, true
	}
	return nil, nil, false
}

// okFracPair reports whether (a, b) are within-magnitude positive fractions with
// different denominators — the MISMATCHED_DENOMINATORS shape.
func okFracPair(a, b *big.Rat, ctx buildCtx) bool {
	if a.Sign() <= 0 || b.Sign() <= 0 || !withinMag(a, ctx) || !withinMag(b, ctx) {
		return false
	}
	return !a.IsInt() && !b.IsInt() && a.Denom().Cmp(b.Denom()) != 0
}

// divisorsInRange returns the integer divisors of n in [2, hi].
func divisorsInRange(n, hi int64) []int64 {
	var out []int64
	for d := int64(2); d <= hi && d <= n; d++ {
		if n%d == 0 {
			out = append(out, d)
		}
	}
	return out
}

// realizeLeaf renders a value as a leaf in an enabled style: percent (n%),
// decimal, fraction, or integer; negative when the value is negative (NEGATIVES).
func realizeLeaf(v *big.Rat, ctx buildCtx) (mathcore.Node, bool) {
	if v.Sign() < 0 && ctx.bitmap&mathcore.NEGATIVES == 0 {
		return nil, false
	}
	if v.IsInt() {
		if !withinMag(v, ctx) {
			return nil, false
		}
		return mathcore.Num{Value: new(big.Rat).Set(v)}, true
	}
	// non-integer: needs a fraction/decimal/percent rendering, which requires the
	// matching concept to be enabled (else it would violate the envelope).
	if ctx.percent && isPercentValue(v) {
		n := new(big.Rat).Mul(v, big.NewRat(100, 1))
		return mathcore.Num{Value: new(big.Rat).Set(v), Raw: mathcore.RatDecimalOrInt(n) + "%", IsPercent: true}, true
	}
	if ctx.decimals && isTerminatingDecimal(v) {
		return mathcore.Num{Value: new(big.Rat).Set(v), Raw: mathcore.RatDecimalOrInt(v), IsDecimal: true}, true
	}
	if ctx.bitmap&mathcore.FRACTIONS != 0 {
		num := v.Num()
		den := v.Denom()
		return mathcore.Num{Value: new(big.Rat).Set(v),
			Raw: num.String() + "/" + den.String(), IsFraction: true}, true
	}
	return nil, false
}

// ---- equation forms ----

// buildVariableEquation builds "<expr in x> = c" with integer solution x. The
// LHS is grown compositionally with the variable substituted for one leaf, so a
// SINGLE_VARIABLE problem still composes other enabled concepts.
func buildVariableEquation(ctx buildCtx) (mathcore.Node, *big.Rat, bool) {
	cap := ctx.operandCap
	k := 2 + ctx.rng.Intn(max(2, min(cap, 12)-1))
	x := 1 + ctx.rng.Intn(max(1, cap/k))
	coeff := mathcore.BinaryExpr{Op: '*', L: numLit(k), R: mathcore.Var{Letter: 'x', HasCoefficient: true}}
	// kx + b = k*x + b, or just kx = kx when no additive op is available.
	op, ok := pickEnabledOp(ctx.bitmap, []byte{'+', '-'}, ctx.rng)
	if !ok {
		rhs := k * x
		return mathcore.Equation{LHS: coeff, RHS: numLit(rhs)}, ri(x), true
	}
	b := 1 + ctx.rng.Intn(cap)
	var c int
	if op == '-' {
		c = k*x - b
		if c < 0 {
			b = 1 + ctx.rng.Intn(max(1, k*x))
			c = k*x - b
		}
	} else {
		c = k*x + b
	}
	lhs := mathcore.BinaryExpr{Op: op, L: coeff, R: numLit(b)}
	return mathcore.Equation{LHS: lhs, RHS: numLit(c)}, ri(x), true
}

// buildMissingEquation builds "<expr with one leaf blanked> = total". The answer
// is the blanked leaf's value; the expression is grown compositionally first.
func buildMissingEquation(ctx buildCtx) (mathcore.Node, *big.Rat, bool) {
	ctx.missing = false // avoid recursion into this branch
	ans := chooseAnswer(ctx)
	node, ok := expand(ans, max(1, ctx.depth), ctx)
	if !ok {
		return nil, nil, false
	}
	be, ok := node.(mathcore.BinaryExpr)
	if !ok {
		return nil, nil, false
	}
	rn, ok := be.R.(mathcore.Num)
	if !ok {
		return nil, nil, false
	}
	total, err := mathcore.Eval(node, nil)
	if err != nil {
		return nil, nil, false
	}
	// The RHS is rendered as a plain Num; a non-integer total would need a
	// fraction/decimal/percent rendering that this leaf does not carry, so skip
	// it rather than emit a malformed equation. The caller retries.
	if !total.IsInt() {
		return nil, nil, false
	}
	answer := new(big.Rat).Set(rn.Value)
	be.R = mathcore.Missing{}
	return mathcore.Equation{LHS: be, RHS: mathcore.Num{Value: total}}, answer, true
}

// ---- fallback ----

// fallback returns a guaranteed-valid, envelope-safe single-operation problem
// when no targeted candidate could be built.
func fallback(bitmap mathcore.ProblemType, rng *rand.Rand) (string, string, error) {
	op := enabledOps(bitmap)[0]
	cap := min(9, bracketCap(bitmap))
	if cap < 2 {
		cap = 2
	}
	switch op {
	case '/':
		b := 2 + rng.Intn(max(1, min(cap, 4)-1))
		q := 1 + rng.Intn(max(1, cap/b))
		return mathcore.Render(mathcore.BinaryExpr{Op: '/', L: numLit(b * q), R: numLit(b)}), itoa(q), nil
	case '*':
		a := 2 + rng.Intn(max(1, min(cap, 4)-1))
		b := 2 + rng.Intn(max(1, cap/a-1))
		return mathcore.Render(mathcore.BinaryExpr{Op: '*', L: numLit(a), R: numLit(b)}), itoa(a * b), nil
	case '-':
		a := 2 + rng.Intn(cap-1)
		b := 1 + rng.Intn(a)
		return mathcore.Render(mathcore.BinaryExpr{Op: '-', L: numLit(a), R: numLit(b)}), itoa(a - b), nil
	default:
		a := 1 + rng.Intn(cap)
		b := 1 + rng.Intn(cap)
		return mathcore.Render(mathcore.BinaryExpr{Op: '+', L: numLit(a), R: numLit(b)}), itoa(a + b), nil
	}
}

// ---- helpers ----

func coreOpsMask(b mathcore.ProblemType) mathcore.ProblemType {
	return b & (mathcore.ADDITION | mathcore.SUBTRACTION | mathcore.MULTIPLICATION | mathcore.DIVISION)
}

func enabledOps(b mathcore.ProblemType) []byte {
	var ops []byte
	if b&mathcore.ADDITION != 0 {
		ops = append(ops, '+')
	}
	if b&mathcore.SUBTRACTION != 0 {
		ops = append(ops, '-')
	}
	if b&mathcore.MULTIPLICATION != 0 {
		ops = append(ops, '*')
	}
	if b&mathcore.DIVISION != 0 {
		ops = append(ops, '/')
	}
	return ops
}

func pickEnabledOp(b mathcore.ProblemType, prefs []byte, rng *rand.Rand) (byte, bool) {
	var avail []byte
	for _, p := range prefs {
		switch p {
		case '+':
			if b&mathcore.ADDITION != 0 {
				avail = append(avail, p)
			}
		case '-':
			if b&mathcore.SUBTRACTION != 0 {
				avail = append(avail, p)
			}
		case '*':
			if b&mathcore.MULTIPLICATION != 0 {
				avail = append(avail, p)
			}
		case '/':
			if b&mathcore.DIVISION != 0 {
				avail = append(avail, p)
			}
		}
	}
	if len(avail) == 0 {
		return 0, false
	}
	return avail[rng.Intn(len(avail))], true
}

func bracketCap(b mathcore.ProblemType) int {
	if b&mathcore.LARGE_NUMBERS != 0 {
		return mathcore.LargeMaxOperand
	}
	if b&mathcore.MEDIUM_NUMBERS != 0 {
		return mathcore.MediumMaxOperand
	}
	return mathcore.SmallMaxOperand
}

// withinMag reports whether a value's digit-magnitude fits the bracket cap (the
// numerator and denominator both within cap, the conservative check that keeps
// detection from stamping an out-of-envelope magnitude bit).
func withinMag(v *big.Rat, ctx buildCtx) bool {
	num := new(big.Int).Abs(v.Num())
	den := v.Denom()
	capN := ctx.maxOperand
	// A bare fraction operand keeps SMALL-bracket components regardless of the
	// magnitude bracket: MEDIUM/LARGE magnitude rides on integer operands
	// (splitDivIntByFrac / splitMulIntWithFrac), not on an inflated numerator.
	// Decimals still scale with the bracket (their magnitude is digit-based).
	if !v.IsInt() && ctx.fractions && !ctx.decimals {
		capN = mathcore.SmallMaxOperand
	}
	cap := big.NewInt(int64(capN))
	return num.Cmp(cap) <= 0 && den.Cmp(cap) <= 0
}

func pickDenom(ctx buildCtx) int {
	cap := min(ctx.maxOperand, 12)
	if cap < 2 {
		cap = 2
	}
	return 2 + ctx.rng.Intn(cap-1)
}

func decimalScale(ctx buildCtx) int {
	if ctx.rng.Intn(2) == 0 {
		return 10
	}
	return 100
}

func isTerminatingDecimal(v *big.Rat) bool {
	d := new(big.Int).Set(v.Denom())
	for _, p := range []int64{2, 5} {
		bp := big.NewInt(p)
		z := new(big.Int)
		for {
			q, r := new(big.Int).DivMod(d, bp, z)
			if r.Sign() != 0 {
				break
			}
			d = q
		}
	}
	return d.Cmp(big.NewInt(1)) == 0
}

func isPercentValue(v *big.Rat) bool {
	x := new(big.Rat).Mul(v, big.NewRat(100, 1))
	return x.IsInt()
}

func ratFloor(v *big.Rat) int64 {
	q := new(big.Int).Quo(v.Num(), v.Denom())
	return q.Int64()
}

func formatAnswer(v *big.Rat, decimal bool) string {
	if v.IsInt() {
		return v.Num().String()
	}
	if decimal && isTerminatingDecimal(v) {
		return mathcore.RatDecimalOrInt(v)
	}
	return v.Num().String() + "/" + v.Denom().String()
}

// ri is a rational from an int (a value); numLit is an AST integer leaf.
func ri(n int) *big.Rat         { return big.NewRat(int64(n), 1) }
func numLit(n int) mathcore.Num { return mathcore.Num{Value: big.NewRat(int64(n), 1)} }

func itoa(n int) string { return big.NewInt(int64(n)).String() }
