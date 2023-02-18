import React, { } from "react";

import './problem.scss'

var ReactFitText = require('react-fittext');

const AttemptTime = ({ timestamp }) => {
  var diff = Math.ceil((Date.now() - Date.parse(timestamp)) / 1000);
  if (diff >= 60) {
    var mins = Math.floor(diff/60);
    if (mins > 1) {
      return <span className="attempt-time">({Math.floor(diff/60)} minutes ago)</span>;
    }
    return <span className="attempt-time">({Math.floor(diff/60)} minute ago)</span>;
  }
  if (diff > 1) {
    return <span className="attempt-time">({diff} seconds ago)</span>;
  }
  return <span className="attempt-time">({diff} second ago)</span>;
}

const ProblemCompanionView = ({ gamestate, latex, answer, attempts }) => {
  if (gamestate == null || latex == null || answer == null || attempts == null) {
    return <div className="content-loading"></div>
  }

  var progress = String(100.0 * gamestate.solved / gamestate.target) + "%";
  return (
    <div id="problem-companion">
      <div id="problem-mirror">
        <div className="progress">
          <div className="progress-meter" style={{width: progress}}>
          </div>
        </div>
        <ReactFitText compressor={0.75}>
            <div id="problem-display" dangerouslySetInnerHTML={{__html: latex}}></div>
        </ReactFitText>
        <div id="problem-answer-companion">
            Answer: {answer}
        </div>
      </div>
      <div id="problem-attempts">
        <div id="problem-attempts-header">
          attempts
        </div>
        {attempts.map((attempt) => (
          <div key={attempt.timestamp} className="problem-attempt">
            {attempt.value} <AttemptTime timestamp={attempt.timestamp} />
          </div>
        ))}
      </div>
    </div>)
}

export {
  ProblemCompanionView
}
