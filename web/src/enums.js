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

// Frontend copy of the event-type constants (server/api/event_types.go).
//
// The client's only event-type vocabulary: bare literals elsewhere in web/src
// are rejected by TestEventTypesMatchJS, which also pins this set to the
// server's. Emitting a client event is documented in docs/gameplay.md.
const EventTypes = {
  LOGGED_IN: "logged_in",
  SELECTED_PROBLEM: "selected_problem",
  WORKING_ON_PROBLEM: "working_on_problem",
  ANSWERED_PROBLEM: "answered_problem",
  SOLVED_PROBLEM: "solved_problem",
  ERROR_PLAYING_VIDEO: "error_playing_video",
  WATCHING_VIDEO: "watching_video",
  DONE_WATCHING_VIDEO: "done_watching_video",
  SET_TARGET_DIFFICULTY: "set_target_difficulty",
  SET_TARGET_WORK_PERCENTAGE: "set_target_work_percentage",
  SET_PROBLEM_TYPE_BITMAP: "set_problem_type_bitmap",
  SET_GAMESTATE_TARGET: "set_gamestate_target",
  BAD_PROBLEM_SYSTEM: "bad_problem_system",
  BAD_PROBLEM_USER: "bad_problem_user",
};

export { ProblemTypes, EventTypes };
