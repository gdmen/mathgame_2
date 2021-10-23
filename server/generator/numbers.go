// Package generator contains a math problem generator
package generator // import "garydmenezes.com/mathgame/server/generator"

import (
	"math"
	"math/big"
	"math/rand"
)

const (
	maxAllowedNumber       = 100000.0
	numberDiffMagnitude    = 3.0
	negativeDiffMultiplier = 1.5
)

func getMaxAllowedNumberDiff(opts *Options) float64 {
	diff := getNumberDiff(maxAllowedNumber)
	if opts.Negatives {
		diff *= negativeDiffMultiplier
	}
	return diff
}

func getNumberDiff(n float64) float64 {
	diff := math.Abs(math.Log(n) / math.Log(numberDiffMagnitude))
	if diff == 0 || math.IsInf(diff, 0) || math.IsNaN(diff) {
		diff = 0.5
	}
	return diff
}

func generateNumber(maxDiff float64, opts *Options) (*big.Rat, float64) {
	// Difficulty is exponentially related to number size
	var denom int64
	denom = 1
	isNeg := false
	if opts.Negatives {
		isNeg = rand.Intn(2) == 0
		if isNeg {
			// Reduce random number diff to account for negative difficulty
			maxDiff /= negativeDiffMultiplier
		}
	}
	if opts.Fractions {
		// Generate non-zero denominator and expand numeratorerator range
		//denom = rand.Int63n(int64(max))
	}

	max := math.Pow(numberDiffMagnitude, maxDiff)
	max = math.Min(max, maxAllowedNumber)

	num := big.NewRat(rand.Int63n(int64(max)), denom)
	numF, _ := num.Float64()
	diff := getNumberDiff(numF)

	if isNeg {
		diff *= negativeDiffMultiplier
		num = num.Neg(num)
	}
	return num, diff
}
