import React, { useEffect, useMemo, useState } from "react";
import parse from "html-react-parser";

import "./problem.scss";

import { EventTypes, ProblemTypes } from "./enums.js";

const PreprocessExpression = (expression) => {
  // KaTeX reads a bare % as a line comment and eats the rest of the math
  // ("70% \div 7" would render as just "70"), so escape any literal percent.
  expression = expression.replace(/(?<!\\)%/g, "\\%");
  // Split each \text{...} block at internal whitespace into per-word
  // \text{} blocks so word-wrap can happen between words.
  expression = expression.replace(/\\text\{[^\}]+\}/g, (match) =>
    match.replace(/\s/g, " }\\text{")
  );
  // Wrap math-mode multi-digit numbers in \text{} so KaTeX renders them
  // as a single atomic span instead of one <span class="mord"> per digit.
  expression = expression.replace(
    /(?<![A-Za-z\\])(\d{2,})(?![A-Za-z])/g,
    "\\text{$1}"
  );
  return expression;
};

class AnswerTracker {
  constructor(eventReporter) {
    var singleton = AnswerTracker._instance;
    if (singleton) {
      // Same handover as EventReporterSingleton: a remount brings a new
      // reporter, and the tracker must post through it, not the dead one.
      singleton.eventReporter = eventReporter;
      return singleton;
    }
    AnswerTracker._instance = this;

    this.eventReporter = eventReporter;
    this.lastAnswer = "";
    this.lastProblemId = null;
    this.answerChanged = false;
  }

  reportAnswer(answer, problem_id) {
    if (answer === "" || answer === this.lastAnswer) {
      return false;
    }
    this.lastAnswer = answer;
    this.lastProblemId = problem_id;
    this.answerChanged = false;
    this.eventReporter.remove(EventTypes.WORKING_ON_PROBLEM);
    this.eventReporter.postEvent(EventTypes.ANSWERED_PROBLEM, answer);
    return true;
  }

  wasIncorrectAnswer(problem_id) {
    var res =
      this.lastAnswer !== "" &&
      !this.answerChanged &&
      this.lastProblemId === problem_id;
    return res;
  }

  answerWasSet() {
    this.answerChanged = true;
  }

  problemWasDisplayed(problem_id) {
    if (this.lastProblemId != problem_id) {
      this.answerChanged = true;
      this.lastAnswer = "";
    }
  }
}

const ProblemView = ({ gamestate, latex, eventReporter, interval }) => {
  const [answer, setAnswer] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const problemId = gamestate ? gamestate.problem_id : null;

  useEffect(() => {
    setAnswer("");
  }, [latex]);

  useEffect(() => {
    setSubmitting(false);
  }, [gamestate]);

  // The tracker is a singleton, so this only ever hands it the live reporter.
  const answerTracker = useMemo(
    () => (eventReporter ? new AnswerTracker(eventReporter) : null),
    [eventReporter]
  );

  // Resetting tracker state is a mutation, so it waits for commit rather than
  // running mid-render.
  useEffect(() => {
    if (answerTracker) answerTracker.problemWasDisplayed(problemId);
  }, [answerTracker, problemId]);

  // "Time on problem" accrues only while a problem is actually on screen and
  // unanswered. Adding on mount and removing on cleanup replaces the old
  // clear-every-render dance, and covers unmount (moving to the video) for
  // free.
  useEffect(() => {
    if (!eventReporter || submitting) return;
    eventReporter.add(EventTypes.WORKING_ON_PROBLEM);
    return () => eventReporter.remove(EventTypes.WORKING_ON_PROBLEM);
  }, [eventReporter, submitting, problemId]);

  if (
    gamestate == null ||
    latex == null ||
    interval == null ||
    answerTracker == null
  ) {
    return <div className="content-loading"></div>;
  }

  var progress = String((100.0 * gamestate.solved) / gamestate.target) + "%";
  // The last problem before the reward switches the meter to $color-reward and
  // announces the video, so the thing being earned is visible while earning it.
  const isFinalProblem = gamestate.target - gamestate.solved === 1;
  return (
    <>
      <div id="problem" className={submitting ? "submitting" : ""}>
        <p className="progress-cue">
          {isFinalProblem ? "1 more until your video" : " "}
        </p>
        <div className={"progress" + (isFinalProblem ? " final" : "")}>
          <div className="progress-meter" style={{ width: progress }}></div>
        </div>
        <div id="problem-display">{parse(latex)}</div>
        <div id="problem-answer" className="input-group">
          <input
            id="problem-answer-input"
            className="input-group-field"
            type="text"
            value={answer}
            readOnly={submitting}
            autoFocus
            onChange={(e) => {
              setAnswer(e.target.value);
              answerTracker.answerWasSet();
            }}
            onKeyDown={(e) => {
              if (e.key === "Enter") {
                !submitting &&
                  setSubmitting(
                    answerTracker.reportAnswer(answer, gamestate.problem_id)
                  );
              }
            }}
          />
          <button
            onClick={() => {
              !submitting &&
                setSubmitting(
                  answerTracker.reportAnswer(answer, gamestate.problem_id)
                );
            }}
          >
            <span className="submit-label">
              <span id="submit-text">submit</span>
              <span className="loader-wrap">
                <span className="loader"></span>
              </span>
            </span>
          </button>
        </div>
        {/* Height is reserved whether or not the nudge shows, so nothing below
            it moves when an answer comes back wrong. Never red (see the
            style guide's colour invariant). */}
        <div className="problem-feedback">
          {!submitting && answerTracker.wasIncorrectAnswer(gamestate.problem_id)
            ? "Try Again!"
            : " "}
        </div>
      </div>
    </>
  );
};

export { ProblemView, PreprocessExpression };
