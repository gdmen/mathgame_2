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

class EventReporterSingleton {
  constructor(postEvent, interval, postAnswer) {
    var singleton = EventReporterSingleton._instance;
    if (singleton) {
      singleton.setUp();
      return singleton;
    }
    EventReporterSingleton._instance = this;

    this.postEvent = postEvent;
    this.interval = interval;
    this.postAnswer = postAnswer;
    this.lastAnswer = "";
    this.lastProblemId = null;
    this.answerChanged = false;

    this.setUp();
  }

  reportAnswer(answer, problem_id) {
    if (answer === "" || answer === this.lastAnswer) {
      return;
    }
    this.tearDown();
    this.lastAnswer = answer;
    this.lastProblemId = problem_id;
    this.answerChanged = false;
    this.postAnswer(answer);
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

  tearDown() {
    window.removeEventListener("focus", this.onFocus);
    window.removeEventListener("blur", this.onBlur);
    clearInterval(this.intervalId);
    this.listenersAlive = false;
    // turn off the reporting loop
    this.onBlur();
  }

  setUp() {
    if (!this.listenersAlive) {
      window.addEventListener("focus", this.onFocus.bind(this));
      window.addEventListener("blur", this.onBlur.bind(this));
      clearInterval(this.intervalId);
      this.intervalId = setInterval(
        this.reportWorking.bind(this),
        this.interval
      );
      this.listenersAlive = true;
    }
    // Call this.onFocus when the window loads
    if (document.hasFocus()) {
      this.onFocus();
    }
  }

  reportWorking() {
    if (this.focus) {
      this.postEvent("working_on_problem", this.interval);
    }
  }

  onFocus() {
    this.focus = true;
  }

  onBlur() {
    this.focus = false;
  }
}

const ProblemView = ({
  gamestate,
  latex,
  isWordProblem,
  postAnswer,
  postEvent,
  interval,
}) => {
  const [answer, setAnswer] = useState("");

  useEffect(() => {
    setAnswer("");
  }, [latex]);

  if (
    gamestate == null ||
    latex == null ||
    isWordProblem == null ||
    postAnswer == null ||
    postEvent == null ||
    interval == null
  ) {
    return <div className="content-loading"></div>;
  }

  var minFontSize = 50;

  postEvent("displayed_problem", gamestate.problem_id);
  var reporter = new EventReporterSingleton(postEvent, interval, postAnswer);
  reporter.problemWasDisplayed(gamestate.problem_id);
  var progress = String((100.0 * gamestate.solved) / gamestate.target) + "%";
  return (
    <>
      <div id="problem">
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
            onChange={(e) => {
              setAnswer(e.target.value);
              reporter.answerWasSet();
            }}
            onKeyDown={(e) => {
              if (e.key === "Enter") {
                reporter.reportAnswer(answer, gamestate.problem_id);
              }
            }}
          />
          <div>
            <button
              onClick={() => {
                reporter.reportAnswer(answer, gamestate.problem_id);
              }}
            >
              <h3>submit</h3>
            </button>
          </div>
        </div>
        {reporter.wasIncorrectAnswer(gamestate.problem_id) && (
          <div className="label alert">
            <div>Try Again!</div>
          </div>
        )}
      </div>
    </>
  );
};

export { ProblemView, IsWordProblem, PreprocessExpression };
