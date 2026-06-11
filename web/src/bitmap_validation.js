// Settings-bitmap validation and the difficulty ceiling.
//
// Part of the problem-generation system - documented in
// docs/problem-generation.md. Behavior changes here (rules, presets,
// ceiling constants) REQUIRE updating that doc in the same PR and must stay
// consistent with the server (server/api/difficulty.go owns the ceiling
// authoritatively - the server clamps on save; this copy only sizes the UI).
import { ProblemTypes } from "./enums.js";

const T = ProblemTypes;

// MIN_TARGET_DIFFICULTY mirrors server/api/difficulty.go MinTargetDifficulty:
// the slider floor (the easiest problems the pool populates score ~3.5).
const MIN_TARGET_DIFFICULTY = 3;

// validateBitmap checks the settings-level dependency rules. Returns
// { valid: true } or { valid: false, errors: [{ code, message, offendingBits }] }.
const validateBitmap = (bitmap) => {
  const errors = [];
  const coreOps = T.ADDITION | T.SUBTRACTION | T.MULTIPLICATION | T.DIVISION;
  if ((bitmap & coreOps) === 0) {
    errors.push({
      code: "NO_CORE_OP",
      message:
        "Pick at least one operation (addition, subtraction, multiplication, or division).",
      offendingBits: [],
    });
  }
  if ((bitmap & T.LARGE_NUMBERS) !== 0 && (bitmap & T.MEDIUM_NUMBERS) === 0) {
    errors.push({
      code: "LARGE_REQUIRES_MEDIUM",
      message:
        "Numbers 100 and up need numbers up to 99 enabled too (no gap in sizes).",
      offendingBits: [T.LARGE_NUMBERS],
    });
  }
  if (
    (bitmap & T.MISMATCHED_DENOMINATORS) !== 0 &&
    (bitmap & T.FRACTIONS) === 0
  ) {
    errors.push({
      code: "MISMATCHED_REQUIRES_FRACTIONS",
      message: "Different denominators need fractions enabled.",
      offendingBits: [T.MISMATCHED_DENOMINATORS],
    });
  }
  if ((bitmap & T.PEMDAS) !== 0 && (bitmap & T.CHAINED_OPERATIONS) === 0) {
    errors.push({
      code: "PEMDAS_REQUIRES_CHAINED",
      message:
        "Order of operations needs multi-step problems enabled (it takes two or more steps to matter).",
      offendingBits: [T.PEMDAS],
    });
  }
  if (errors.length > 0) {
    return { valid: false, errors: errors };
  }
  return { valid: true };
};

// maxDiffForBitmap mirrors server/api/difficulty.go MaxDiffForBitmap: the
// difficulty of the hardest problem the enabled bits can express. Used to
// size the target-difficulty slider (the slider range IS the envelope).
// Either/or rule: MISSING_NUMBER and SINGLE_VARIABLE are per-problem
// mutually exclusive, so the ceiling takes the higher branch.
const maxDiffForBitmap = (bitmap) => {
  let maxOperand = 12;
  if ((bitmap & T.MEDIUM_NUMBERS) !== 0) maxOperand = 99;
  if ((bitmap & T.LARGE_NUMBERS) !== 0) maxOperand = 9999;
  const magnitude = Math.log10(maxOperand + 1) + 0.3;

  let opWeight = 1.0;
  if ((bitmap & T.SUBTRACTION) !== 0) opWeight = Math.max(opWeight, 1.1);
  if ((bitmap & T.MULTIPLICATION) !== 0) opWeight = Math.max(opWeight, 2.2);
  if ((bitmap & T.DIVISION) !== 0) opWeight = Math.max(opWeight, 2.8);

  let concept = 1.0;
  if ((bitmap & T.FRACTIONS) !== 0) concept *= 2.0;
  if ((bitmap & T.MISMATCHED_DENOMINATORS) !== 0) concept *= 1.5;
  if ((bitmap & T.NEGATIVES) !== 0) concept *= 1.3;
  if ((bitmap & T.WORD) !== 0) concept *= 1.3;
  if ((bitmap & T.PEMDAS) !== 0) concept *= 1.5;
  if ((bitmap & T.DECIMALS) !== 0) concept *= 2.0;
  if ((bitmap & T.PERCENTAGES) !== 0) concept *= 2.0;

  let structure = 1.0;
  if ((bitmap & T.CHAINED_OPERATIONS) !== 0) structure = 1.0 + 0.15 * 4; // MaxChainLen 5

  const compress = (raw) => {
    const scaled =
      1 +
      (19 * (Math.log(raw + 1) - Math.log(1.5))) /
        (Math.log(16) - Math.log(1.5));
    return Math.max(1, scaled);
  };

  let best = magnitude * opWeight * concept * structure;
  if ((bitmap & T.SINGLE_VARIABLE) !== 0) {
    best = Math.max(best, magnitude * opWeight * concept * 5.0 * structure);
  }
  if ((bitmap & T.MISSING_NUMBER) !== 0) {
    best = Math.max(best, magnitude * opWeight * concept * (structure + 0.2));
  }
  return compress(best);
};

export { validateBitmap, maxDiffForBitmap, MIN_TARGET_DIFFICULTY };
