// Frontend copy of the ProblemType bit constants (server/api/enums.go).
//
// Part of the problem-generation system - documented in
// docs/problem-generation.md. New bits MUST be added here, in the server
// enums, AND in that doc (the new-bit checklist) in the same PR.
const ProblemTypes = {
  ADDITION: Math.pow(2, 0),
  SUBTRACTION: Math.pow(2, 1),
  MULTIPLICATION: Math.pow(2, 2),
  DIVISION: Math.pow(2, 3),
  FRACTIONS: Math.pow(2, 4),
  NEGATIVES: Math.pow(2, 5),
  WORD: Math.pow(2, 6),
  MEDIUM_NUMBERS: Math.pow(2, 7),
  LARGE_NUMBERS: Math.pow(2, 8),
  CHAINED_OPERATIONS: Math.pow(2, 9),
  MISSING_NUMBER: Math.pow(2, 10),
  MISMATCHED_DENOMINATORS: Math.pow(2, 11),
  DECIMALS: Math.pow(2, 12),
  PEMDAS: Math.pow(2, 13),
  SINGLE_VARIABLE: Math.pow(2, 14),
  PERCENTAGES: Math.pow(2, 15),
};

export { ProblemTypes };
