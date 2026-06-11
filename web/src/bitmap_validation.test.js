import { validateBitmap, maxDiffForBitmap } from "./bitmap_validation.js";
import { ProblemTypes as T } from "./enums.js";

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
      validateBitmap(T.ADDITION | T.MEDIUM_NUMBERS | T.LARGE_NUMBERS).valid
    ).toBe(true);
  });

  it("MISMATCHED requires FRACTIONS", () => {
    const res = validateBitmap(T.ADDITION | T.MISMATCHED_DENOMINATORS);
    expect(res.valid).toBe(false);
    expect(res.errors.map((e) => e.code)).toContain(
      "MISMATCHED_REQUIRES_FRACTIONS"
    );
  });

  it("PEMDAS requires CHAINED_OPERATIONS", () => {
    const res = validateBitmap(T.ADDITION | T.PEMDAS);
    expect(res.valid).toBe(false);
    expect(res.errors.map((e) => e.code)).toContain("PEMDAS_REQUIRES_CHAINED");
    expect(
      validateBitmap(T.ADDITION | T.CHAINED_OPERATIONS | T.PEMDAS).valid
    ).toBe(true);
  });

  it("accepts a minimal valid bitmap", () => {
    expect(validateBitmap(T.ADDITION).valid).toBe(true);
  });
});

describe("maxDiffForBitmap (UI mirror of server MaxDiffForBitmap)", () => {
  // Reference ceilings from the server's TestMaxDiffForBitmap tables - if
  // these drift, the Go constants changed and this mirror must follow.
  it("matches the server reference ceilings", () => {
    expect(maxDiffForBitmap(T.ADDITION | T.SUBTRACTION)).toBeCloseTo(5.28, 1);
    expect(
      maxDiffForBitmap(T.ADDITION | T.SUBTRACTION | T.MISSING_NUMBER)
    ).toBeCloseTo(6.2, 1);
    expect(
      maxDiffForBitmap(T.ADDITION | T.SUBTRACTION | T.SINGLE_VARIABLE)
    ).toBeCloseTo(15.18, 1);
    const all = Object.values(T).reduce((a, b) => a | b, 0);
    expect(maxDiffForBitmap(all)).toBeCloseTo(61.82, 1);
  });

  it("the slider max grows with the envelope (dynamic max)", () => {
    const small = maxDiffForBitmap(T.ADDITION);
    const bigger = maxDiffForBitmap(
      T.ADDITION | T.MULTIPLICATION | T.MEDIUM_NUMBERS
    );
    expect(bigger).toBeGreaterThan(small);
  });
});
