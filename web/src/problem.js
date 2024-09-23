import React, { useEffect, useState } from "react";

import "./problem.scss";

var ReactFitText = require("react-fittext");

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
    return (
      this.lastAnswer !== "" &&
      !this.answerChanged &&
      this.lastProblemId === problem_id
    );
  }

  answerWasSet() {
    this.answerChanged = true;
  }

  newProblemWasDisplayed() {
    this.answerChanged = true;
    this.lastAnswer = "";
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

const ProblemView = ({ gamestate, latex, postAnswer, postEvent, interval }) => {
  const [answer, setAnswer] = useState("");

  useEffect(() => {
    setAnswer("");
  }, [latex]);

  if (
    gamestate == null ||
    latex == null ||
    postAnswer == null ||
    postEvent == null ||
    interval == null
  ) {
    return <div className="content-loading"></div>;
  }

  postEvent("displayed_problem", gamestate.problem_id);
  var reporter = new EventReporterSingleton(postEvent, interval, postAnswer);
  reporter.newProblemWasDisplayed();
  var progress = String((100.0 * gamestate.solved) / gamestate.target) + "%";
  return (
    <>
      <div id="problem">
        <div className="progress">
          <div className="progress-meter" style={{ width: progress }}></div>
        </div>
        <ReactFitText compressor={0.75}>
          <div
            id="problem-display"
            dangerouslySetInnerHTML={{ __html: latex }}
          ></div>
        </ReactFitText>
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

export { ProblemView };
