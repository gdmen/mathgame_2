// Package generator contains a math problem generator
package generator // import "garydmenezes.com/mathgame/server/generator"

import (
	"fmt"
	"math"
	"math/big"
	"math/rand"
	"regexp"
	//"github.com/golang/glog"
)

const (
	startingDifficultyRatio = 1 / 3.0
	targetDelta             = 0.25
)

type OptionsError struct {
	s string
}

func (e *OptionsError) Error() string {
	return e.s
}

type Problem struct {
	Expr     string
	ans      *big.Rat
	isNumber bool
	Diff     float64
}

func (p *Problem) String() string {
	desc := "Problem"
	if p.isNumber {
		desc = "Number"
	}
	return fmt.Sprintf("%s[%f] {%s = %s}", desc, p.Diff, p.ExprString(), p.AnsString())
}

func (p *Problem) ExprString() string {
	re := regexp.MustCompile(`\s+`)
	p.Expr = re.ReplaceAllString(p.Expr, "")
	return p.Expr
}

func (p *Problem) AnsString() string {
	if p.ans == nil {
		return ""
	}
	return p.ans.RatString()
}

func (p *Problem) GetAns() *big.Rat {
	if p.ans == nil {
		return nil
	}
	r := big.NewRat(0, 1)
	return r.Set(p.ans)
}

func (p *Problem) SetAns(a *big.Rat) {
	if p.ans == nil {
		p.ans = big.NewRat(0, 1)
	}
	p.ans.Set(a)
}

// Return <bigger>, <smaller>
func SortProblems(a *Problem, b *Problem) (*Problem, *Problem) {
	if b.GetAns().Cmp(a.GetAns()) > 0 {
		return b, a
	}
	return a, b
}

type Options struct {
	Operations       []string `json:"operations" form:"operations"`
	Fractions        bool     `json:"fractions" form:"fractions"`
	Negatives        bool     `json:"negatives" form:"negatives"`
	TargetDifficulty float64  `json:"target_difficulty" form:"target_difficulty"`
}

func (opts Options) String() string {
	return fmt.Sprintf("Operations: %s", opts.Operations)
}

func validateOptions(opts *Options) error {
	for _, o := range opts.Operations {
		if !isSupportedOperation(o) {
			return &OptionsError{s: fmt.Sprintf("'%s' is not a supported operation", o)}
		}
	}
	return nil
}

func generateNumberProblem(maxDifficulty float64, opts *Options) *Problem {
	num, diff := generateNumber(maxDifficulty, opts)
	p := &Problem{
		isNumber: true,
		Diff:     diff,
	}
	p.SetAns(num)
	p.Expr = p.AnsString()
	return p
}

func GenerateProblem(opts *Options) (string, string, float64, error) {
	err := validateOptions(opts)
	if err != nil {
		return "", "", 0, err
	}

	prev := generateNumberProblem(opts.TargetDifficulty*startingDifficultyRatio, opts)

	iter := 1
	curr := doStep(prev, iter, opts)
	for prev.Expr != curr.Expr {
		prev = curr
		iter += 1
		curr = doStep(prev, iter, opts)
	}

	return curr.ExprString(), curr.AnsString(), curr.Diff, nil
}

func doStep(a *Problem, iteration int, opts *Options) *Problem {
	//logPrefix := "[doStep]"
	//glog.Infof("%s fcn start", logPrefix)
	remainDiff := opts.TargetDifficulty - a.Diff
	//glog.Infof("%s iteration: %d\n", logPrefix, iteration)
	//glog.Infof("%s remainDiff: %f\n", logPrefix, remainDiff)
	//glog.Infof("%s min reasonable: %f\n", logPrefix, getNumberDiff(opts.TargetDifficulty/math.Log(float64(iteration))))
	if iteration > 1 && remainDiff <= getNumberDiff(opts.TargetDifficulty/math.Log(float64(iteration))) {
		//glog.Infof("%s returning_a: %s\n", logPrefix, a)
		return a
	}
	possOps := []Operation{}
	for _, v := range opts.Operations {
		possOps = append(possOps, operations[v])
	}
	for len(possOps) > 0 {
		i := rand.Intn(len(possOps))
		randOp := possOps[i]
		maxBDiff := randOp.getInputDiff(remainDiff)
		b := generateNumberProblem(maxBDiff, opts)
		n := randOp.do(a, b, opts)
		//glog.Infof("%s randOp %s: %s\n", logPrefix, randOp.String(), n)
		if n.Diff-opts.TargetDifficulty <= targetDelta {
			return n
		}
		// Remove randOp
		possOps[i] = possOps[len(possOps)-1]
		possOps = possOps[:len(possOps)-1]
	}
	//glog.Infof("%s returning_z: %s\n", logPrefix, a)
	return a
}
