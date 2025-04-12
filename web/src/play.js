import katex from "katex";
import React, { useCallback, useEffect, useState } from "react";

import "katex/dist/katex.min.css";

import { ProblemView, PreprocessExpression } from "./problem.js";
import { VideoView } from "./video.js";
import { ClearSessionPin } from "./pin.js";

const conf = require("./conf");

class EventReporterSingleton {
  constructor(postEvent, interval) {
    var singleton = EventReporterSingleton._instance;
    if (singleton) {
      singleton.setUp();
      return singleton;
    }
    EventReporterSingleton._instance = this;
    this.intervalId = null;
    this.events = new Set();

    this.postEvent = postEvent;
    this.interval = interval;

    this.setUp();
  }

  add(event_type) {
    this.events.add(event_type);
  }

  remove(event_type) {
    this.events.delete(event_type);
  }

  clear() {
    this.events.clear();
  }

  executeInterval() {
    if (!this.focus) {
      return;
    }
    this.events.forEach(
      function (event_type) {
        this.postEvent(event_type, this.interval);
      }.bind(this)
    );
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
        this.executeInterval.bind(this),
        this.interval
      );
      this.listenersAlive = true;
    }
    // Call this.onFocus when the window loads
    if (document.hasFocus()) {
      this.onFocus();
    }
  }

  onFocus() {
    this.focus = true;
  }

  onBlur() {
    this.focus = false;
  }
}

const PlayView = ({ token, apiUrl, user, postEvent, interval }) => {
  const [gamestate, setGamestate] = useState(null);
  const [problem, setProblem] = useState(null);
  const [latex, setLatex] = useState(null);
  const [video, setVideo] = useState(null);

  ClearSessionPin();

  useEffect(() => {
    const getPlayData = async () => {
      try {
        if (token == null || apiUrl == null || user == null) {
          return;
        }
        var reqParams = {
          method: "GET",
          headers: {
            Accept: "application/json",
            "Content-Type": "application/json",
            Authorization: "Bearer " + token,
          },
        };
        var req = await fetch(apiUrl + "/play/" + user.id, reqParams);
        const json = await req.json();
        setGamestate(json["gamestate"]);
        setProblem(json["problem"]);
        setVideo(json["video"]);
      } catch (e) {
        console.log(e.message);
      }
    };

    getPlayData();
  }, [token, apiUrl, user]);

  useEffect(() => {
    const renderLatex = async () => {
      try {
        if (gamestate == null || problem == null) {
          return;
        }
        setLatex(
          katex.renderToString(PreprocessExpression(problem.expression))
        );
      } catch (e) {
        console.log(e.message);
        // handle rendering error
        postEvent("bad_problem_system", gamestate.problem_id).then(() => {
          window.location.pathname = "play";
        });
      }
    };

    renderLatex();
  }, [gamestate, problem]);

  const eventReporter = new EventReporterSingleton(
    async (event_type, value) => {
      let json = await postEvent(event_type, value);
      if (event_type == "answered_problem") {
        setGamestate(json["gamestate"]);
        setProblem(json["problem"]);
        setVideo(json["video"]);
      }
    },
    interval
  );
  eventReporter.clear();

  if (!gamestate || !problem) {
    return <div className="content-loading"></div>;
  }

  if (gamestate.solved >= gamestate.target) {
    if (conf.debug_quickplay) {
      postEvent("watching_video", 5000).then(() => {
        postEvent("done_watching_video", gamestate.video_id).then(() => {
          window.location.pathname = "play";
        });
      });
      return null;
    } else {
      return (
        <VideoView
          video={video}
          eventReporter={eventReporter}
          interval={interval}
        />
      );
    }
  } else {
    if (conf.debug_quickplay) {
      postEvent("working_on_problem", 1000).then(() => {
        postEvent("answered_problem", problem.answer).then(() => {
          window.location.pathname = "play";
        });
      });
      return null;
    } else {
      return (
        <ProblemView
          gamestate={gamestate}
          latex={latex}
          eventReporter={eventReporter}
          interval={interval}
        />
      );
    }
  }
};

export { PlayView };
