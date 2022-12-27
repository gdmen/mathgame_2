import katex from 'katex';
import React, { useCallback, useEffect, useState } from "react";
import { useParams } from 'react-router-dom'

import "katex/dist/katex.min.css"

import { ProblemCompanionView } from './problem_companion.js'

class RefresherSingleton {
  constructor(getGamestate, getEvents, interval) {
    var singleton = RefresherSingleton._instance;
    if (singleton) {
      singleton.setUpListeners();
      return singleton;
    }
    RefresherSingleton._instance = this;

    this.getGamestate = getGamestate;
    this.getEvents = getEvents;
    this.interval = interval;

    this.setUpListeners();

    setInterval(this.refreshData.bind(this), this.interval);
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

  refreshData() {
      if (this.focus) {
        this.getGamestate();
        this.getEvents();
      }
  }

  onFocus() {
    this.focus = true;
  }

  onBlur() {
    this.focus = false;
  }
}

const CompanionView = ({ token, url, user }) => {
  const [gamestate, setGamestate] = useState(null);
  const [problem, setProblem] = useState(null);
  const [answer, setAnswer] = useState(null);
  const [attempts, setAttempts] = useState(null);
  const [latex, setLatex] = useState(null);
  const { student_id } = useParams();
  const interval = 1000;

  //TODO: add a loop to re-fetch gamestate and events.

  const getGamestate = useCallback(async () => {
    try {
      if (token == null || url == null || student_id == null) {
        return;
      }
      const settings = {
          method: 'GET',
          headers: {
            'Accept': 'application/json',
            'Content-Type': 'application/json',
            'Authorization': 'Bearer ' + token,
          },
      };
      const req = await fetch(url+ "/gamestates/" + student_id, settings);
      const json = await req.json();
      setGamestate(json);
    } catch (e) {
      console.log(e.message);
    }
  }, [token, url, student_id]);

  const getProblem = useCallback(async () => {
    try {
      if (token == null || url == null || gamestate == null) {
        return;
      }
      const settings = {
          method: 'GET',
          headers: {
            'Accept': 'application/json',
            'Content-Type': 'application/json',
            'Authorization': 'Bearer ' + token,
          },
      };
      const req = await fetch(url+ "/problems/" + gamestate.problem_id, settings);
      const json = await req.json();
      setProblem(json);
      setAnswer(json["answer"]);
      setLatex(katex.renderToString(json.expression));
    } catch (e) {
      console.log(e.message);
    }
  }, [token, url, gamestate]);

  const getEvents = useCallback(async () => {
    try {
      if (token == null || url == null || student_id == null || gamestate == null) {
        return;
      }
      const settings = {
          method: 'GET',
          headers: {
            'Accept': 'application/json',
            'Content-Type': 'application/json',
            'Authorization': 'Bearer ' + token,
          },
      };
      const req = await fetch(url+ "/events/" + student_id + "/" + 3000, settings);
      const json = await req.json();

      // Clean up, sort, and store events
      var attempts = [];
      var attempts_buffer = [];

      for (var i = json.length - 1; i >= 0; i--) {
        var e = json[i];
        if (e.event_type === "answered_problem") {
          attempts_buffer.push(e);
        } else if (e.event_type === "displayed_problem") {
          if (e.value !== gamestate.problem_id.toString()) {
            break;
          }
          if (attempts_buffer.length > 0) {
            attempts = attempts.concat(attempts_buffer);
          }
          attempts_buffer = [];
        }
      }
      console.log(attempts);
      setAttempts(attempts);
    } catch (e) {
      console.log(e.message);
    }
  }, [token, url, student_id, gamestate]);

  useEffect(() => {
    getGamestate();
  }, [getGamestate]);

  useEffect(() => {
    getProblem();
  }, [getProblem]);

  useEffect(() => {
    getEvents();
  }, [getEvents]);

  if (!gamestate || !problem) {
    return <div id="loading">loading</div>
  }

  new RefresherSingleton(getGamestate, getEvents, interval);

  if (gamestate.solved >= gamestate.target) {
    return <div>watching video</div>
  }
  return <ProblemCompanionView gamestate={gamestate} latex={latex} answer={answer} attempts={attempts}/>
}

export {
  CompanionView
}