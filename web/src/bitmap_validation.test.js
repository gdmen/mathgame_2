import {
  validateBitmap,
  maxDiffForBitmap,
  minDiffForBitmap,
  targetDifficultyRange,
} from "./bitmap_validation.js";
import { ProblemTypes as T } from "./enums.generated.js";
import bandFixtures from "./difficulty_band_fixtures.json";

describe("validateBitmap", () => {
  it("requires at least one core operation", () => {
    const res = validateBitmap(T.FRACTIONS | T.DECIMALS);
    expect(res.valid).toBe(false);
    expect(res.errors.map((e) => e.code)).toContain("NO_CORE_OP");
  });

  it("LARGE requires MEDIUM (no size gap)", () => {
    const res = validateBitmap(T.ADDITION | T.LARGE_NUMBERS);
    expect(res.valid).toBe(false);
    expect(res.errors.map((e) => e.code)).toContain("LARGE_REQUIRES_MEDIUM");
    expect(
      validateBitmap(T.ADDITION | T.MEDIUM_NUMBERS | T.LARGE_NUMBERS).valid,
    ).toBe(true);
  });

  it("MISMATCHED requires FRACTIONS", () => {
    const res = validateBitmap(T.ADDITION | T.MISMATCHED_DENOMINATORS);
    expect(res.valid).toBe(false);
    expect(res.errors.map((e) => e.code)).toContain(
      "MISMATCHED_REQUIRES_FRACTIONS",
    );
  });

  it("PEMDAS requires CHAINED_OPERATIONS", () => {
    const res = validateBitmap(T.ADDITION | T.PEMDAS);
    expect(res.valid).toBe(false);
    expect(res.errors.map((e) => e.code)).toContain("PEMDAS_REQUIRES_CHAINED");
    expect(
      validateBitmap(T.ADDITION | T.CHAINED_OPERATIONS | T.PEMDAS).valid,
    ).toBe(true);
  });

  it("PERCENTAGES require MULTIPLICATION and MEDIUM (percent-of is a two-digit multiplication)", () => {
    const res = validateBitmap(T.ADDITION | T.PERCENTAGES);
    expect(res.valid).toBe(false);
    expect(res.errors.map((e) => e.code)).toContain(
      "PERCENTAGES_REQUIRE_MULTIPLICATION",
    );
    expect(res.errors.map((e) => e.code)).toContain(
      "PERCENTAGES_REQUIRE_MEDIUM",
    );
    expect(validateBitmap(T.MULTIPLICATION | T.PERCENTAGES).valid).toBe(false);
    expect(
      validateBitmap(T.MULTIPLICATION | T.MEDIUM_NUMBERS | T.PERCENTAGES).valid,
    ).toBe(true);
  });

  it("accepts a minimal valid bitmap", () => {
    expect(validateBitmap(T.ADDITION).valid).toBe(true);
  });
});

describe("difficulty band mirrors (maxDiffForBitmap / minDiffForBitmap)", () => {
  // The fixtures are generated FROM the Go source of truth
  // (make gen-difficulty-fixtures -> cmd/gen_difficulty_fixtures); the Go
  // side pins them via TestDifficultyBandFixturesSync (server/api). Any
  // drift between the Go formula and this hand-maintained mirror fails
  // here at full float precision - not at a hand-copied 1-decimal snapshot.
  it("matches the generated server fixtures exactly", () => {
    expect(bandFixtures.length).toBeGreaterThan(0);
    const failures = [];
    for (const f of bandFixtures) {
      const gotMax = maxDiffForBitmap(f.bits);
      const gotMin = minDiffForBitmap(f.bits);
      const { lo, hi } = targetDifficultyRange(f.bits);
      if (Math.abs(gotMax - f.max) > 1e-9) {
        failures.push(`${f.name} (bits=${f.bits}): max ${gotMax} != ${f.max}`);
      }
      if (Math.abs(gotMin - f.min) > 1e-9) {
        failures.push(`${f.name} (bits=${f.bits}): min ${gotMin} != ${f.min}`);
      }
      if (Math.abs(lo - f.lo) > 1e-9 || Math.abs(hi - f.hi) > 1e-9) {
        failures.push(
          `${f.name} (bits=${f.bits}): range [${lo}, ${hi}] != [${f.lo}, ${f.hi}]`,
        );
      }
    }
    expect(failures).toEqual([]);
  });

  it("the slider max grows with the envelope (dynamic max)", () => {
    const small = maxDiffForBitmap(T.ADDITION);
    const bigger = maxDiffForBitmap(
      T.ADDITION | T.MULTIPLICATION | T.MEDIUM_NUMBERS,
    );
    expect(bigger).toBeGreaterThan(small);
  });

  it("targetDifficultyRange floors at the global minimum or the envelope floor", () => {
    // Addition floors at the global minimum (its envelope floor is lower).
    expect(targetDifficultyRange(T.ADDITION).lo).toBe(3);
    // Division-only floors at its envelope floor (~7): nothing easier is
    // constructible under the x2.8 op weight.
    const div = targetDifficultyRange(T.DIVISION);
    expect(div.lo).toBeGreaterThan(3);
    expect(div.lo).toBeLessThan(div.hi);
    expect(div.lo).toBe(Math.max(3, minDiffForBitmap(T.DIVISION)));
  });
});
