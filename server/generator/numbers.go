package generator // import "garydmenezes.com/mathgame/server/generator"

import (
	"math/rand"
)

// randIntRange returns a random int in [min, max] inclusive.
// Both min and max are included.
func randIntRange(min, max int) int {
	if max < min {
		return min
	}
	return rand.Intn(max-min+1) + min
}

// randNonZeroInRange returns a random non-zero int in [min, max] inclusive.
// Useful for denominators and divisors.
func randNonZeroInRange(min, max int) int {
	for i := 0; i < 10; i++ {
		n := randIntRange(min, max)
		if n != 0 {
			return n
		}
	}
	if min <= 0 && max >= 1 {
		return 1
	}
	return min
}

// gcd returns the greatest common divisor of a and b.
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
	if a == 0 {
		return 1
	}
	return a
}

// lcm returns the least common multiple of a and b.
func lcm(a, b int) int {
	return a * b / gcd(a, b)
}
