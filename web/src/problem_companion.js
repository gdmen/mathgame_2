import React from "react";
import { Textfit } from "react-textfit";
import parse from "html-react-parser";

import "./problem.scss";

const AttemptTime = ({ timestamp }) => {
  var diff = Math.ceil((Date.now() - Date.parse(timestamp)) / 1000);
  if (diff >= 60) {
    var mins = Math.floor(diff / 60);
    if (mins > 1) {
      return (
        <span className="attempt-time">
          ({Math.floor(diff / 60)} minutes ago)
        </span>
      );
    }
    return (
      <span className="attempt-time">({Math.floor(diff / 60)} minute ago)</span>
    );
  }
  if (diff > 1) {
    return <span className="attempt-time">({diff} seconds ago)</span>;
  }
  return <span className="attempt-time">({diff} second ago)</span>;
};

const ProblemCompanionView = ({ gamestate, latex, answer, attempts }) => {
  if (
    gamestate == null ||
    latex == null ||
    answer == null ||
    attempts == null
  ) {
    return <div className="content-loading"></div>;
  }

  var progress = String((100.0 * gamestate.solved) / gamestate.target) + "%";
  return (
    <div id="problem-companion">
      <div id="problem-mirror">
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
        <div id="problem-answer-companion">Answer: {answer}</div>
      </div>
      <div id="problem-attempts">
        <div id="problem-attempts-header">attempts</div>
        {attempts.map((attempt) => (
          <div key={attempt.timestamp} className="problem-attempt">
            {attempt.value} <AttemptTime timestamp={attempt.timestamp} />
          </div>
        ))}
      </div>
    </div>
  );
};

export { ProblemCompanionView };
