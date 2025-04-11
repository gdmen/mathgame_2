import katex from "katex";
import React, { useCallback, useEffect, useState } from "react";
import { useParams } from "react-router-dom";

import "katex/dist/katex.min.css";

import { PreprocessExpression } from "./problem.js";
import { ProblemCompanionView } from "./problem_companion.js";
import { VideoCompanionView } from "./video_companion.js";
import { RequirePin } from "./pin.js";

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
  const [video, setVideo] = useState(null);
  const [latex, setLatex] = useState(null);

  const [answer, setAnswer] = useState(null);
  const [attempts, setAttempts] = useState(null);
  const { student_id } = useParams();
  const interval = 30000;

  const getGamestate = useCallback(async () => {
    try {
      if (token == null || url == null || student_id == null) {
        return;
      }
      const reqParams = {
        method: "GET",
        headers: {
          Accept: "application/json",
          "Content-Type": "application/json",
          Authorization: "Bearer " + token,
        },
      };
      const req = await fetch(url + "/gamestates/" + student_id, reqParams);
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
      const reqParams = {
        method: "GET",
        headers: {
          Accept: "application/json",
          "Content-Type": "application/json",
          Authorization: "Bearer " + token,
        },
      };
      const req = await fetch(
        url + "/problems/" + gamestate.problem_id,
        reqParams
      );
      const json = await req.json();
      setProblem(json);
      setAnswer(json["answer"]);
      setLatex(katex.renderToString(PreprocessExpression(json.expression)));
    } catch (e) {
      console.log(e.message);
    }
  }, [token, url, gamestate]);

  const getVideo = useCallback(async () => {
    try {
      if (token == null || url == null || gamestate == null) {
        return;
      }
      const reqParams = {
        method: "GET",
        headers: {
          Accept: "application/json",
          "Content-Type": "application/json",
          Authorization: "Bearer " + token,
        },
      };
      const req = await fetch(url + "/videos/" + gamestate.video_id, reqParams);
      const json = await req.json();
      setVideo(json);
    } catch (e) {
      console.log(e.message);
    }
  }, [token, url, gamestate]);

  const getEvents = useCallback(async () => {
    try {
      if (
        token == null ||
        url == null ||
        student_id == null ||
        gamestate == null
      ) {
        return;
      }
      const reqParams = {
        method: "GET",
        headers: {
          Accept: "application/json",
          "Content-Type": "application/json",
          Authorization: "Bearer " + token,
        },
      };
      const req = await fetch(
        url + "/events/" + student_id + "/" + 3000,
        reqParams
      );
      const json = await req.json();

      // Clean up, sort, and store events
      var attempts = [];
      var attempts_buffer = [];

      for (var i = json.length - 1; i >= 0; i--) {
        var e = json[i];
        if (e.event_type === "answered_problem") {
          attempts_buffer.push(e);
        } else if (e.event_type === "selected_problem") {
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
    getVideo();
  }, [getVideo]);

  useEffect(() => {
    getEvents();
  }, [getEvents]);

  if (!RequirePin(user.id)) {
    return <div className="content-loading"></div>;
  }

  if (!gamestate || !problem) {
    return <div className="content-loading"></div>;
  }

  new RefresherSingleton(getGamestate, getEvents, interval);

  if (gamestate.solved >= gamestate.target) {
    return <VideoCompanionView video={video} />;
  }
  return (
    <ProblemCompanionView
      gamestate={gamestate}
      latex={latex}
      answer={answer}
      attempts={attempts}
    />
  );
};

export { CompanionView };
