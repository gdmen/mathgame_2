import React, { useEffect, useState } from "react";
import { Textfit } from "react-textfit";
import parse from "html-react-parser";

import "./problem.scss";

import { ProblemTypes } from "./enums.js";

const IsWordProblem = (problem) => {
  return Boolean(problem.problem_type_bitmap & ProblemTypes.WORD);
};
const PreprocessExpression = (expression) => {
  function replacer(match, offset, string) {
    return match.replace(/\s/g, " }\\text{");
  }
  return expression.replace(/\\text\{[^\}]+\}/g, replacer);
};

class AnswerTracker {
  constructor(eventReporter) {
    var singleton = AnswerTracker._instance;
    if (singleton) {
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
    this.eventReporter.remove("working_on_problem");
    this.eventReporter.postEvent("answered_problem", answer);
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

const ProblemView = ({
  gamestate,
  latex,
  isWordProblem,
  eventReporter,
  interval,
}) => {
  const [answer, setAnswer] = useState("");
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    setAnswer("");
  }, [latex]);

  useEffect(() => {
    setSubmitting(false);
  }, [gamestate]);

  if (
    gamestate == null ||
    latex == null ||
    isWordProblem == null ||
    interval == null
  ) {
    return <div className="content-loading"></div>;
  }

  var minFontSize = 50;

  var answerTracker = new AnswerTracker(eventReporter);
  answerTracker.problemWasDisplayed(gamestate.problem_id);
  if (!submitting) {
    console.log("adding working_on_problem");
    eventReporter.add("working_on_problem");
  }
  var progress = String((100.0 * gamestate.solved) / gamestate.target) + "%";
  return (
    <>
      <div id="problem" className={submitting ? "submitting" : ""}>
        <div className="progress">
          <div className="progress-meter" style={{ width: progress }}></div>
        </div>
        <div
          id="problem-display"
          className={isWordProblem ? "word-problem" : ""}
        >
          <Textfit mode="multi" min={minFontSize}>
            {parse(latex)}
          </Textfit>
        </div>
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
          <div>
            <button
              onClick={() => {
                !submitting &&
                  setSubmitting(
                    answerTracker.reportAnswer(answer, gamestate.problem_id)
                  );
              }}
            >
              <h3>
                <span id="submit-text">submit</span>
                <span className="loader-wrap">
                  <span className="loader"></span>
                </span>
              </h3>
            </button>
          </div>
        </div>
        {!submitting &&
          answerTracker.wasIncorrectAnswer(gamestate.problem_id) && (
            <div className="label alert">
              <div>Try Again!</div>
            </div>
          )}
      </div>
    </>
  );
};

export { ProblemView, IsWordProblem, PreprocessExpression };
