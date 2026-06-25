package generator

// heuristic2.go: the heuristic_2.0 builder — an answer-first, difficulty-
// targeting compositional generator on the mathcore render-only AST.
//
// Strategy: an inverter biases each candidate toward the target (RawForDifficulty
// gives a raw_target; magnitude is solved from the chosen op/concept/structure
// product), but TARGETING CORRECTNESS comes from generate-and-select — every
// candidate is rendered, run through the canonical mathcore pipeline
// (AdmitExpression + VerifyAnswerSymbolic + DetectProblemTypeBitmap +
// ComputeProblemDifficulty), and the closest in-window survivor is returned.
// Everything fails closed: a construction bug costs a retry, never a wrong or
// out-of-envelope problem. A near-ceiling constructibility gap degrades to the
// closest achievable problem (and ultimately a deterministic fallback), never a
// spin.
//
// Minimal-concept policy: magnitude and chain length are the near-continuous
// primary dials; concepts are coarse x2-x5 jumps. Each attempt assumes a random
// magnitude headroom and adds the fewest, cheapest concepts that move the
// raw_target product closest (in log space), so across attempts the
// generate-and-select loop sees both concept-light and concept-heavy candidates
// and keeps the closest.

import (
	"math"
	"math/big"
	"math/rand"

	"garydmenezes.com/mathgame/server/mathcore"
)

// VERSION is the generator version string stamped on created problems.
// See docs/generator-versions.md for version history.
const VERSION = "heuristic_2.0"

// OptionsError is returned when the requested envelope cannot produce a problem
// (e.g. no core operation enabled).
type OptionsError struct{ s string }

func (e *OptionsError) Error() string { return e.s }

// targetWindow is the half-width of the difficulty window a candidate must land
// in to be accepted immediately. It mirrors selection's additive epsilon
// (problemSelectionEpsilon) so a hit here is a hit at serving time.
const targetWindow = 1.5

// buildAttempts bounds the generate-and-select loop per call.
const buildAttempts = 200

// buildConfig is one candidate shape the inverter proposes; construct turns it
// into a concrete AST + answer.
type buildConfig struct {
	bitmap     mathcore.ProblemType
	ops        []byte // operators for the numeric-chain modes
	operandCap int    // largest operand to place (sets magnitude)
	// concept selections (each a subset of the enabled bitmap)
	fractions, mismatched, negatives, decimals, percent, variable, missing, pemdas bool
}

// GenerateProblem (heuristic_2.0) builds a symbolic problem for the given
// envelope bitmap aimed at target difficulty. Returns (expression, answer,
// error). The heuristic emits only symbolic problems (no WORD), so there is no
// symbolic_expression. See docs/generator-versions.md.
func BuildProblem(bitmap mathcore.ProblemType, target float64, rng *rand.Rand) (string, string, error) {
	if coreOpsMask(bitmap) == 0 {
		return "", "", &OptionsError{s: "no core operation enabled in bitmap"}
	}
	rawTarget := mathcore.RawForDifficulty(target)

	var bestExpr, bestAns string
	bestErr := math.MaxFloat64

	for attempt := 0; attempt < buildAttempts; attempt++ {
		cfg := sampleConfig(bitmap, rawTarget, rng)
		node, unknownAns, ok := construct(cfg, rng)
		if !ok {
			continue
		}
		expr := mathcore.Render(node)
		adm := mathcore.AdmitExpression(expr)
		if adm.RejectStage != "" {
			continue
		}
		// Answer-by-evaluation: for a plain expression the answer is the exact
		// value of the rendered tokens (eliminates any tree-vs-render mismatch);
		// for an equation it is the unknown the construction solved for.
		var ansVal *big.Rat
		if unknownAns != nil {
			ansVal = unknownAns
		} else {
			v, err := mathcore.EvalTokens(adm.Tokens, mathcore.Binding{})
			if err != nil {
				continue
			}
			ansVal = v
		}
		// A negative answer for a no-negatives envelope is a pedagogical leak even
		// though it stamps no NEGATIVES token; skip it.
		if bitmap&mathcore.NEGATIVES == 0 && ansVal.Sign() < 0 {
			continue
		}
		ans := formatAnswer(ansVal, cfg.decimals || cfg.percent)
		if mathcore.VerifyAnswerSymbolic(adm.Tokens, ans) != nil {
			continue
		}
		bm := mathcore.NormalizeProblemBitmap(adm.Bitmap)
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
		// Closest achievable: the cell's constructible difficulty does not reach
		// the target window (a coarse-concept or near-ceiling gap). A valid
		// in-envelope problem, just easier/harder than asked — never a spin.
		return bestExpr, bestAns, nil
	}
	return fallback(bitmap, rng)
}

// ---- envelope helpers ----

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

func opBit(op byte) mathcore.ProblemType {
	switch op {
	case '+':
		return mathcore.ADDITION
	case '-':
		return mathcore.SUBTRACTION
	case '*':
		return mathcore.MULTIPLICATION
	case '/':
		return mathcore.DIVISION
	}
	return 0
}

// pickOp returns a random enabled op from the preference list, ok=false if none
// of them are enabled.
func pickEnabledOp(b mathcore.ProblemType, prefs []byte, rng *rand.Rand) (byte, bool) {
	var avail []byte
	for _, p := range prefs {
		if b&opBit(p) != 0 {
			avail = append(avail, p)
		}
	}
	if len(avail) == 0 {
		return 0, false
	}
	return avail[rng.Intn(len(avail))], true
}

// additiveOp returns an enabled additive operator ('+' preferred, then '-').
func additiveOp(b mathcore.ProblemType, rng *rand.Rand) (byte, bool) {
	return pickEnabledOp(b, []byte{'+', '-'}, rng)
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

// ---- the inverter (config sampling) ----

// sampleConfig proposes a candidate shape biased toward rawTarget. Each attempt
// assumes a random magnitude headroom in [magMin, magCeil]; the residual sets
// how many concepts to add (cheapest, log-closest first) and what operand
// magnitude to solve for. The randomized assumption spreads candidates across
// the concept/magnitude tradeoff so generate-and-select can keep the closest.
func sampleConfig(bitmap mathcore.ProblemType, rawTarget float64, rng *rand.Rand) buildConfig {
	cap := bracketCap(bitmap)
	cfg := buildConfig{bitmap: bitmap}

	cfg.ops = chooseChainOps(bitmap, rng)
	opW := opWeight(cfg.ops)

	numOps := len(cfg.ops)
	structure := 1.0 + mathcore.StructurePerExtraOp*float64(numOps-1)
	// Missing-number is a per-problem unknown, mutually exclusive with
	// SINGLE_VARIABLE: flag it here, and construct ignores it on the attempts
	// where the concept chooser also selects a variable (variable wins). This
	// way a both-enabled envelope still produces missing-number problems on the
	// (lower-target) attempts that don't reach for the variable.
	if bitmap&mathcore.MISSING_NUMBER != 0 && rng.Intn(3) == 0 {
		cfg.missing = true
		structure += mathcore.StructureMissing
	}

	const magMin = 0.6 // magnitude of a tiny operand (log10(2)+0.3)
	magCeil := math.Log10(float64(cap)+1) + 0.3
	assumedMag := magMin + rng.Float64()*(magCeil-magMin)
	need := rawTarget / (opW * structure * assumedMag)
	cfg.chooseConcepts(need, rng)

	conceptF := conceptFactor(cfg)
	magNeeded := rawTarget / (opW * structure * conceptF)
	operandCap := math.Pow(10, magNeeded-0.3) - 1
	operandCap *= math.Exp(rng.NormFloat64() * 0.2) // multiplicative noise for spread
	cfg.operandCap = clampOperandCap(int(math.Round(operandCap)), bitmap, cap, rng)
	return cfg
}

// chooseChainOps picks the operator sequence for the numeric-chain modes,
// honoring PEMDAS availability (mixing additive with multiplicative without
// PEMDAS would trip the precedence detector and waste the candidate).
func chooseChainOps(bitmap mathcore.ProblemType, rng *rand.Rand) []byte {
	ops := enabledOps(bitmap)
	pick := ops[rng.Intn(len(ops))]
	if bitmap&mathcore.CHAINED_OPERATIONS == 0 {
		return []byte{pick}
	}
	n := 1 + rng.Intn(mathcore.MaxChainLen)
	additive := pick == '+' || pick == '-'
	var pool []byte
	if additive {
		for _, o := range ops {
			if o == '+' || o == '-' {
				pool = append(pool, o)
			}
		}
	} else if pick == '*' {
		pool = []byte{'*'}
	} else { // '/'
		return []byte{'/'} // division kept single-op for clean construction
	}
	out := make([]byte, n)
	for i := range out {
		out[i] = pool[rng.Intn(len(pool))]
	}
	return out
}

// conceptOpt is one addable concept: its bit, multiplier, usability, and setter.
type conceptOpt struct {
	bit    mathcore.ProblemType
	mult   float64
	usable bool
	set    func(*buildConfig)
}

// chooseConcepts greedily adds the enabled, usable concept that moves the
// running product closest to need (in log space), stopping when no concept
// improves the fit. This is the minimal-concept policy: cheap concepts are
// preferred, and a concept whose jump would overshoot need more than the
// current shortfall is never added (the difficulty window absorbs the residual).
func (cfg *buildConfig) chooseConcepts(need float64, rng *rand.Rand) {
	b := cfg.bitmap
	hasAdditive := b&(mathcore.ADDITION|mathcore.SUBTRACTION) != 0
	hasMul := b&mathcore.MULTIPLICATION != 0
	opts := []conceptOpt{
		{mathcore.NEGATIVES, mathcore.ConceptNegatives, b&mathcore.NEGATIVES != 0,
			func(c *buildConfig) { c.negatives = true }},
		{mathcore.PEMDAS, mathcore.ConceptPEMDAS,
			b&mathcore.PEMDAS != 0 && b&mathcore.CHAINED_OPERATIONS != 0 && (hasMul || b&mathcore.SUBTRACTION != 0),
			func(c *buildConfig) { c.pemdas = true }},
		{mathcore.DECIMALS, mathcore.ConceptDecimals, b&mathcore.DECIMALS != 0 && hasAdditive,
			func(c *buildConfig) { c.decimals = true }},
		{mathcore.PERCENTAGES, mathcore.ConceptPercent, b&mathcore.PERCENTAGES != 0 && (hasMul || hasAdditive),
			func(c *buildConfig) { c.percent = true }},
		{mathcore.FRACTIONS, mathcore.ConceptFractions, b&mathcore.FRACTIONS != 0 && (hasAdditive || hasMul),
			func(c *buildConfig) { c.fractions = true }},
		{mathcore.SINGLE_VARIABLE, mathcore.ConceptVariable, b&mathcore.SINGLE_VARIABLE != 0,
			func(c *buildConfig) { c.variable = true }},
	}
	// Shuffle so equal-cost concepts (e.g. DECIMALS/PERCENTAGES/FRACTIONS, all
	// x2.0) get fair selection; the greedy max-improvement loop still prefers a
	// cheaper concept when it lands closer to need, so the minimal-concept policy
	// holds — shuffling only breaks ties.
	rng.Shuffle(len(opts), func(i, j int) { opts[i], opts[j] = opts[j], opts[i] })
	got := 1.0
	used := map[mathcore.ProblemType]bool{}
	logDist := func(v float64) float64 { return math.Abs(math.Log(need) - math.Log(v)) }
	for {
		bestIdx := -1
		bestImprove := 0.0
		for i, o := range opts {
			if !o.usable || used[o.bit] {
				continue
			}
			improve := logDist(got) - logDist(got*o.mult)
			if improve > bestImprove {
				bestImprove = improve
				bestIdx = i
			}
		}
		if bestIdx < 0 {
			break // no concept gets us closer
		}
		o := opts[bestIdx]
		o.set(cfg)
		used[o.bit] = true
		got *= o.mult
		// MISMATCHED stacks on FRACTIONS when it helps and is enabled.
		if o.bit == mathcore.FRACTIONS && b&mathcore.MISMATCHED_DENOMINATORS != 0 {
			if logDist(got*mathcore.ConceptMismatched) < logDist(got) {
				cfg.mismatched = true
				got *= mathcore.ConceptMismatched
			}
		}
	}
}

func conceptFactor(cfg buildConfig) float64 {
	f := 1.0
	if cfg.fractions {
		f *= mathcore.ConceptFractions
		if cfg.mismatched {
			f *= mathcore.ConceptMismatched
		}
	}
	if cfg.negatives {
		f *= mathcore.ConceptNegatives
	}
	if cfg.variable {
		f *= mathcore.ConceptVariable
	}
	if cfg.pemdas {
		f *= mathcore.ConceptPEMDAS
	}
	if cfg.decimals {
		f *= mathcore.ConceptDecimals
	}
	if cfg.percent {
		f *= mathcore.ConceptPercent
	}
	return f
}

func opWeight(ops []byte) float64 {
	w := 1.0
	for _, o := range ops {
		switch o {
		case '-':
			w = math.Max(w, mathcore.WeightSub)
		case '*':
			w = math.Max(w, mathcore.WeightMul)
		case '/':
			w = math.Max(w, mathcore.WeightDiv)
		}
	}
	return w
}

// clampOperandCap keeps the solved operand within an envelope-safe magnitude
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
	if cap > mathcore.SmallMaxOperand && bitmap&mathcore.MEDIUM_NUMBERS == 0 && bitmap&mathcore.LARGE_NUMBERS == 0 {
		cap = mathcore.SmallMaxOperand
	}
	if cap > mathcore.MediumMaxOperand && bitmap&mathcore.LARGE_NUMBERS == 0 {
		cap = mathcore.MediumMaxOperand
	}
	return cap
}

// ---- construction (answer-first) ----

// construct turns a config into a concrete AST. The second return is the
// unknown's value for an EQUATION (variable / missing); it is nil for a plain
// expression, signaling the caller to take the answer by evaluating the render.
func construct(cfg buildConfig, rng *rand.Rand) (mathcore.Node, *big.Rat, bool) {
	switch {
	case cfg.variable:
		return buildVariableEq(cfg, rng)
	case cfg.percent:
		n, ok := buildPercent(cfg, rng)
		return n, nil, ok
	case cfg.fractions:
		n, ok := buildFractionChain(cfg, rng)
		return n, nil, ok
	default:
		node, ok := buildNumericChain(cfg, rng)
		if !ok {
			return nil, nil, false
		}
		if cfg.missing {
			return wrapMissing(node)
		}
		return node, nil, true
	}
}

// evalNode renders, lexes, and evaluates a node to its exact value (the helper
// behind answer-by-evaluation for the missing-number construction).
func evalNode(n mathcore.Node) (*big.Rat, bool) {
	toks, lexErr := mathcore.LexExpression(mathcore.NormalizeExpression(mathcore.Render(n)))
	if lexErr != nil {
		return nil, false
	}
	v, err := mathcore.EvalTokens(toks, mathcore.Binding{})
	if err != nil {
		return nil, false
	}
	return v, true
}

func randRange(rng *rand.Rand, lo, hi int) int {
	if hi < lo {
		hi = lo
	}
	return lo + rng.Intn(hi-lo+1)
}

// buildNumericChain builds an integer chain (decimals delegate to a decimal
// sum). PEMDAS configs build a precedence shape where correct != naive;
// otherwise a same-precedence chain. Division is single-op and exact. The answer
// is taken by evaluating the render, so only the NODE is returned.
func buildNumericChain(cfg buildConfig, rng *rand.Rand) (mathcore.Node, bool) {
	if cfg.decimals {
		return buildDecimalSum(cfg, rng)
	}
	cap := cfg.operandCap
	if cap < 2 {
		cap = 2
	}

	if cfg.pemdas {
		return buildPemdas(cfg, cap, rng)
	}

	ops := cfg.ops
	if len(ops) == 0 {
		ops = []byte{'+'}
	}
	if len(ops) == 1 && ops[0] == '/' {
		b := randRange(rng, 2, max(2, min(cap, 12)))
		q := randRange(rng, 1, max(1, cap/b))
		a := b * q
		return mathcore.BinaryExpr{Op: '/', L: numLit(a), R: numLit(b)}, true
	}

	first := randRange(rng, 1, cap)
	var node mathcore.Node = numLit(first)
	acc := first // running value to bound subtraction when negatives are off
	for _, op := range ops {
		switch op {
		case '-':
			hi := cap
			if !cfg.negatives && acc < hi {
				hi = acc
			}
			if hi < 1 {
				hi = 1
			}
			b := randRange(rng, 1, hi)
			node = mathcore.BinaryExpr{Op: '-', L: node, R: numLit(b)}
			acc -= b
		case '*':
			b := randRange(rng, 2, max(2, min(cap, 12)))
			node = mathcore.BinaryExpr{Op: '*', L: node, R: numLit(b)}
			acc *= b
		default: // '+'
			b := randRange(rng, 1, cap)
			node = mathcore.BinaryExpr{Op: '+', L: node, R: numLit(b)}
			acc += b
		}
	}
	if cfg.negatives {
		node = withNegativeLead(node, cfg.bitmap, cap, rng)
	}
	return node, true
}

// buildPemdas builds a precedence-sensitive expression (correct != naive).
// "a + b*c" when multiplication is enabled, else "a - (b - c)" with subtraction.
func buildPemdas(cfg buildConfig, cap int, rng *rand.Rand) (mathcore.Node, bool) {
	if cfg.bitmap&mathcore.MULTIPLICATION != 0 {
		a := randRange(rng, 1, cap)
		b := randRange(rng, 1, cap)
		c := randRange(rng, 2, max(2, min(cap, 12)))
		var node mathcore.Node = mathcore.BinaryExpr{Op: '+',
			L: numLit(a),
			R: mathcore.BinaryExpr{Op: '*', L: numLit(b), R: numLit(c)}}
		if cfg.negatives {
			node = withNegativeLead(node, cfg.bitmap, cap, rng)
		}
		return node, true
	}
	if cfg.bitmap&mathcore.SUBTRACTION != 0 {
		c := randRange(rng, 1, cap)
		b := randRange(rng, c+1, c+cap) // b>c keeps (b-c) positive
		a := randRange(rng, b, b+cap)   // a>=b keeps a-(b-c) positive
		node := mathcore.BinaryExpr{Op: '-',
			L: numLit(a),
			R: mathcore.Paren{X: mathcore.BinaryExpr{Op: '-', L: numLit(b), R: numLit(c)}}}
		return node, true
	}
	return nil, false
}

// withNegativeLead prefixes a negative operand via an enabled additive operator
// so a NEGATIVES token is present; the answer is taken by evaluating the render.
// With no additive op enabled the lead can't be placed cleanly, so the node is
// returned unchanged (that attempt simply won't carry a negative).
func withNegativeLead(node mathcore.Node, bitmap mathcore.ProblemType, cap int, rng *rand.Rand) mathcore.Node {
	op, ok := additiveOp(bitmap, rng)
	if !ok {
		return node
	}
	a := randRange(rng, 1, cap)
	return mathcore.BinaryExpr{Op: op, L: mathcore.Num{Value: big.NewRat(int64(-a), 1)}, R: node}
}

// buildDecimalSum builds "x.y +/- z.w" with one or two decimal places, exact,
// using an enabled additive operator.
func buildDecimalSum(cfg buildConfig, rng *rand.Rand) (mathcore.Node, bool) {
	op, ok := additiveOp(cfg.bitmap, rng)
	if !ok {
		return nil, false
	}
	scale := int64([]int{10, 100}[rng.Intn(2)])
	cap := cfg.operandCap
	if cap < 1 {
		cap = 1
	}
	hi := max(1, cap*int(scale)/10)
	an := int64(randRange(rng, 1, hi))
	bn := int64(randRange(rng, 1, hi))
	if op == '-' && bn > an {
		an, bn = bn, an // keep the difference non-negative
	}
	node := mathcore.BinaryExpr{Op: op,
		L: mathcore.Num{Value: big.NewRat(an, scale), IsDecimal: true},
		R: mathcore.Num{Value: big.NewRat(bn, scale), IsDecimal: true}}
	return node, true
}

// buildFractionChain builds "a/d +/- b/d" (same denom) or "a/d1 +/- b/d2"
// (mismatched). Literal denominators are pinned via Raw so reduction can't
// collapse them. Uses an enabled additive operator.
func buildFractionChain(cfg buildConfig, rng *rand.Rand) (mathcore.Node, bool) {
	op, ok := additiveOp(cfg.bitmap, rng)
	if !ok {
		return nil, false
	}
	cap := cfg.operandCap
	dmax := max(2, min(cap, 12))
	d1 := randRange(rng, 2, dmax)
	a := randRange(rng, 1, max(1, d1-1))
	d2 := d1
	if cfg.mismatched {
		if dmax <= 2 {
			return nil, false
		}
		d2 = randRange(rng, 2, dmax)
		for d2 == d1 {
			d2 = randRange(rng, 2, dmax)
		}
	}
	b := randRange(rng, 1, max(1, d2-1))
	if op == '-' && big.NewRat(int64(a), int64(d1)).Cmp(big.NewRat(int64(b), int64(d2))) < 0 {
		a, d1, b, d2 = b, d2, a, d1 // keep the difference non-negative
	}
	node := mathcore.BinaryExpr{Op: op, L: fracNum(a, d1), R: fracNum(b, d2)}
	return node, true
}

// buildPercent builds "n% * base" (answer-first whole product) when MUL is
// enabled, else "n% +/- m%" with an additive operator.
func buildPercent(cfg buildConfig, rng *rand.Rand) (mathcore.Node, bool) {
	nicePct := []int{10, 20, 25, 50, 75, 5, 40, 60, 80}
	cap := cfg.operandCap
	if cfg.bitmap&mathcore.MULTIPLICATION != 0 {
		n := nicePct[rng.Intn(len(nicePct))]
		if cap < 4 {
			cap = 12
		}
		step := 100 / gcd(n, 100)
		maxK := max(1, cap/step)
		base := step * randRange(rng, 1, maxK)
		return mathcore.BinaryExpr{Op: '*', L: pctNum(n), R: numLit(base)}, true
	}
	op, ok := additiveOp(cfg.bitmap, rng)
	if !ok {
		return nil, false
	}
	n := nicePct[rng.Intn(len(nicePct))]
	m := nicePct[rng.Intn(len(nicePct))]
	if op == '-' && m > n {
		n, m = m, n // keep the difference non-negative
	}
	return mathcore.BinaryExpr{Op: op, L: pctNum(n), R: pctNum(m)}, true
}

// buildVariableEq builds "k x +/- b = c" (or "k x = c" with no additive op),
// with integer solution x (the answer).
func buildVariableEq(cfg buildConfig, rng *rand.Rand) (mathcore.Node, *big.Rat, bool) {
	cap := cfg.operandCap
	if cap < 3 {
		cap = 12
	}
	k := randRange(rng, 2, max(2, min(cap, 12)))
	x := randRange(rng, 1, max(1, cap/k))
	coeff := mathcore.BinaryExpr{Op: '*', L: numLit(k), R: mathcore.Var{Letter: 'x', HasCoefficient: true}}
	op, ok := additiveOp(cfg.bitmap, rng)
	if !ok {
		// No additive op: "k x = c".
		node := mathcore.Equation{LHS: coeff, RHS: numLit(k * x)}
		return node, big.NewRat(int64(x), 1), true
	}
	b := randRange(rng, 1, cap)
	var c int
	if op == '-' {
		c = k*x - b
		if c < 0 {
			b = randRange(rng, 1, max(1, k*x))
			c = k*x - b
		}
	} else {
		c = k*x + b
	}
	lhs := mathcore.BinaryExpr{Op: op, L: coeff, R: numLit(b)}
	node := mathcore.Equation{LHS: lhs, RHS: numLit(c)}
	return node, big.NewRat(int64(x), 1), true
}

// wrapMissing turns a numeric chain into an equation with its right leaf
// blanked; the answer becomes that leaf's value, and the equation's RHS is the
// chain's evaluated total.
func wrapMissing(node mathcore.Node) (mathcore.Node, *big.Rat, bool) {
	be, ok := node.(mathcore.BinaryExpr)
	if !ok {
		return nil, nil, false
	}
	rn, ok := be.R.(mathcore.Num)
	if !ok {
		return nil, nil, false
	}
	total, ok := evalNode(node)
	if !ok {
		return nil, nil, false
	}
	answer := new(big.Rat).Set(rn.Value)
	be.R = mathcore.Missing{}
	eq := mathcore.Equation{LHS: be, RHS: mathcore.Num{Value: total}}
	return eq, answer, true
}

// fallback returns a guaranteed-valid, envelope-safe simple problem when no
// targeted candidate could be built: a single operation with operands inside
// the magnitude bracket (so no out-of-envelope magnitude bit is stamped).
func fallback(bitmap mathcore.ProblemType, rng *rand.Rand) (string, string, error) {
	op := enabledOps(bitmap)[0]
	cap := min(9, bracketCap(bitmap))
	if cap < 2 {
		cap = 2
	}
	switch op {
	case '/':
		b := randRange(rng, 2, max(2, min(cap, 4)))
		q := randRange(rng, 1, max(1, cap/b))
		node := mathcore.BinaryExpr{Op: '/', L: numLit(b * q), R: numLit(b)}
		return mathcore.Render(node), itoa(q), nil
	case '*':
		a := randRange(rng, 2, max(2, min(cap, 4)))
		b := randRange(rng, 2, max(2, cap/a))
		return mathcore.Render(mathcore.BinaryExpr{Op: '*', L: numLit(a), R: numLit(b)}), itoa(a * b), nil
	case '-':
		a := randRange(rng, 2, cap)
		b := randRange(rng, 1, a)
		return mathcore.Render(mathcore.BinaryExpr{Op: '-', L: numLit(a), R: numLit(b)}), itoa(a - b), nil
	default:
		a := randRange(rng, 1, cap)
		b := randRange(rng, 1, cap)
		return mathcore.Render(mathcore.BinaryExpr{Op: '+', L: numLit(a), R: numLit(b)}), itoa(a + b), nil
	}
}

// ---- small helpers ----

func numLit(n int) mathcore.Num { return mathcore.Num{Value: big.NewRat(int64(n), 1)} }

func pctNum(n int) mathcore.Num {
	return mathcore.Num{Value: big.NewRat(int64(n), 100), Raw: itoa(n) + "%", IsPercent: true}
}

func fracNum(n, d int) mathcore.Num {
	return mathcore.Num{Value: big.NewRat(int64(n), int64(d)), Raw: itoa(n) + "/" + itoa(d), IsFraction: true}
}

func formatAnswer(v *big.Rat, decimal bool) string {
	if v.IsInt() {
		return v.Num().String()
	}
	if decimal {
		s := v.FloatString(6)
		for len(s) > 0 && s[len(s)-1] == '0' {
			s = s[:len(s)-1]
		}
		if len(s) > 0 && s[len(s)-1] == '.' {
			s = s[:len(s)-1]
		}
		return s
	}
	return v.Num().String() + "/" + v.Denom().String()
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func gcd(a, b int) int {
	if a < 0 {
		a = -a
	}
	if b < 0 {
		b = -b
	}
	for b != 0 {
		a, b = b, a%b
	}
	return a
}

func itoa(n int) string { return big.NewInt(int64(n)).String() }
