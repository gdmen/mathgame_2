import katex from "katex";
import React, { useCallback, useEffect, useState } from "react";

import "katex/dist/katex.min.css";

import { ProblemView, IsWordProblem, PreprocessExpression } from "./problem.js";
import { VideoView } from "./video.js";
import { ClearSessionPin } from "./pin.js";

const conf = require("./conf");

class EventReporterSingleton {
  constructor(postEvent) {
    var singleton = EventReporterSingleton._instance;
    if (singleton) {
      singleton.setUp();
      return singleton;
    }
    EventReporterSingleton._instance = this;

    this.postEvent = postEvent;
    // events to report in the format {event_type:interval}
    this.events = new Map();
    this.intervalIds = [];

    this.setUp();
  }

  // Add an event to be sent on an interval
  add(event_type, interval) {
    this.events.set(event_type, interval);
    this.tearDown();
    this.setUp();
  }

  // Clear all intervals
  clear() {
    this.events.clear();
    this.tearDown();
    this.setUp();
  }

  tearDown() {
    window.removeEventListener("focus", this.onFocus);
    window.removeEventListener("blur", this.onBlur);
    this.intervalIds.forEach((i) => {
      clearInterval(i);
    });
    this.intervalIds = [];
    this.listenersAlive = false;
    // turn off the reporting loop
    this.onBlur();
  }

  setUp() {
    if (!this.listenersAlive) {
      window.addEventListener("focus", this.onFocus.bind(this));
      window.addEventListener("blur", this.onBlur.bind(this));
      this.intervalIds.forEach((i) => {
        clearInterval(i);
      });
      for (let [event_type, interval] of this.events) {
        this.intervalIds.push(
          setInterval(
            this.genIntervalEventFcn(event_type, interval).bind(this),
            interval
          )
        );
      }
      this.listenersAlive = true;
    }
    // Call this.onFocus when the window loads
    if (document.hasFocus()) {
      this.onFocus();
    }
  }

  genIntervalEventFcn(event_type, interval) {
    return function () {
      if (this.focus) {
        this.postEvent(event_type, interval);
      }
    };
  }

  onFocus() {
    this.focus = true;
  }

  onBlur() {
    this.focus = false;
  }
}

const PlayView = ({ token, url, user, postEvent, interval }) => {
  const [gamestate, setGamestate] = useState(null);
  const [problem, setProblem] = useState(null);
  const [latex, setLatex] = useState(null);
  const [video, setVideo] = useState(null);

  ClearSessionPin();

  const getGamestate = useCallback(async () => {
    try {
      if (token == null || url == null || user == null) {
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
      const req = await fetch(url + "/gamestates/" + user.id, reqParams);
      const json = await req.json();
      setGamestate(json);
    } catch (e) {
      console.log(e.message);
    }
  }, [token, url, user]);

  const getProblem = useCallback(async () => {
    var json = null;
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
      json = await req.json();
      setProblem(json);
    } catch (e) {
      console.log(e.message);
    }
    try {
      setLatex(katex.renderToString(PreprocessExpression(json.expression)));
    } catch (e) {
      console.log(e.message);
      // handle rendering error
      postEvent("bad_problem_system", gamestate.problem_id).then(() => {
        window.location.pathname = "play";
      });
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

  useEffect(() => {
    getGamestate();
  }, [getGamestate]);

  useEffect(() => {
    getProblem();
  }, [getProblem]);

  useEffect(() => {
    getVideo();
  }, [getVideo]);

  const eventReporter = new EventReporterSingleton(
    async (event_type, value) => {
      let gamestate = await postEvent(event_type, value);
      if (event_type == "answered_problem") {
        setGamestate(gamestate);
      }
    }
  );

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
      eventReporter.clear();
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
          isWordProblem={IsWordProblem(problem)}
          eventReporter={eventReporter}
          interval={interval}
        />
      );
    }
  }
};

export { PlayView };
