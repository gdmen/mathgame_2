import React, { useEffect, useState } from "react";

import './problem.css'

var ReactFitText = require('react-fittext');

class EventReporterSingleton {
  constructor(postEvent, interval, postAnswer) {
    var singleton = EventReporterSingleton._instance;
    if (singleton) {
      singleton.setUpListeners();
      return singleton;
    }
    EventReporterSingleton._instance = this;

    this.postEvent = postEvent;
    this.interval = interval;
    this.postAnswer = postAnswer;
    this.lastAnswer = "";

    this.setUpListeners();

    setInterval(this.reportWorking.bind(this), this.interval);
  }

  reportAnswer(answer) {
    if (answer === "" || answer === this.lastAnswer) {
      return;
    }
    this.lastAnswer = answer;
    this.tearDownListeners();
    this.postAnswer(answer);
  }

  tearDownListeners() {
    window.removeEventListener("focus", this.onFocus);
    window.removeEventListener("blur", this.onBlur);
    this.listenersAlive = false;
    // turn off the reporting loop
    this.onBlur();
  }

  setUpListeners() {
    if (!this.listenersAlive) {
      window.addEventListener("focus", this.onFocus.bind(this));
      window.addEventListener("blur", this.onBlur.bind(this));
      this.listenersAlive = true;
    }
    // Call this.onFocus when the window loads
    this.onFocus();
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

  if (gamestate == null || latex == null || postAnswer == null || postEvent == null || interval == null) {
    return <div id="loading"></div>
  }

  postEvent("displayed_problem", gamestate.problem_id);
  var reporter = new EventReporterSingleton(postEvent, interval, postAnswer);
  var progress = String(100.0 * gamestate.solved / gamestate.target) + "%";
  return (<>
    <div className="success progress">
      <div className="progress-meter" style={{width: progress}}></div>
    </div>
    <div id="problem">
        <ReactFitText compressor={0.75}>
            <div id="problem-display" dangerouslySetInnerHTML={{__html: latex}}></div>
        </ReactFitText>
        <div id="problem-answer" className="input-group">
            <input id="problem-answer-input" className="input-group-field" type="text"
                value={answer}
                onChange={(e) => { setAnswer(e.target.value); }}
                onKeyDown={(e) => { if (e.key === "Enter") { reporter.reportAnswer(answer); }}}
            />
            <div className="input-group-button">
                <input type="submit" className="button" value="answer"
                  onClick={() => { reporter.reportAnswer(answer); }}
                />
            </div>
        </div>
    </div>
  </>)
}

export {
  ProblemView
}
