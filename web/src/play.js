import katex from "katex";
import React, { useEffect, useRef, useState } from "react";

import "katex/dist/katex.min.css";

import { ProblemView, PreprocessExpression } from "./problem.js";
import { VideoView } from "./video.js";
import { PinConfirmModal } from "./pin_confirm_modal.js";

import "./play.scss";

const REPORT_EXPLANATION_MAX_LENGTH = 500;

const conf = require("./conf");

class EventReporterSingleton {
  constructor(postEvent, interval) {
    var singleton = EventReporterSingleton._instance;
    if (singleton) {
      // A remount hands over the new view's reporter, or the instance would
      // keep calling the unmounted one's callback.
      singleton.postEvent = postEvent;
      singleton.interval = interval;
      singleton.setUp();
      return singleton;
    }
    EventReporterSingleton._instance = this;
    this.intervalId = null;
    this.events = new Set();

    this.postEvent = postEvent;
    this.interval = interval;
    // Bound once and stored: removeEventListener only matches the same
    // reference, so binding per-call would leak a listener on every teardown.
    this.onFocus = this.onFocus.bind(this);
    this.onBlur = this.onBlur.bind(this);
    this.executeInterval = this.executeInterval.bind(this);

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
      window.addEventListener("focus", this.onFocus);
      window.addEventListener("blur", this.onBlur);
      clearInterval(this.intervalId);
      this.intervalId = setInterval(this.executeInterval, this.interval);
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
  const [showReportModal, setShowReportModal] = useState(false);
  const [reportExplanation, setReportExplanation] = useState("");
  const [reportError, setReportError] = useState("");
  const [reportSubmitting, setReportSubmitting] = useState(false);

  useEffect(() => {
    // The 403 branch navigates the whole document, so a response landing
    // after unmount must be dropped: the view that asked is gone, and the
    // view that replaced it (the setup wizard, when the gate catches up) must
    // not be torn down by its predecessor's answer.
    let cancelled = false;
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
        const text = await req.text();
        if (cancelled) {
          return;
        }
        if (!req.ok) {
          if (req.status === 403) {
            window.location.pathname = "/";
          }
          return;
        }
        if (!text || text.trim() === "") {
          console.log("Play API returned empty body");
          return;
        }
        let json;
        try {
          json = JSON.parse(text);
        } catch (parseErr) {
          console.log("Play API invalid JSON: " + parseErr.message);
          return;
        }
        setGamestate(json["gamestate"]);
        setProblem(json["problem"]);
        setVideo(json["video"]);
      } catch (e) {
        console.log(e.message);
      }
    };

    getPlayData();
    return () => {
      cancelled = true;
    };
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
        const value = JSON.stringify({
          problem_id: gamestate.problem_id,
          explanation: e.message || "LaTeX rendering failed",
        });
        postEvent("bad_problem_system", value).then((json) => {
          if (json && json.gamestate) {
            setGamestate(json.gamestate);
            setProblem(json.problem);
            setVideo(json.video);
          } else {
            window.location.pathname = "play";
          }
        });
      }
    };

    renderLatex();
  }, [gamestate, problem, postEvent]);

  // postEvent is a fresh function on every parent render (index.js calls
  // genPostEventFcn()), so the reporter reads it through a ref instead of being
  // rebuilt — rebuilding would tear down and re-arm the reporting interval
  // constantly.
  const postEventRef = useRef(postEvent);
  useEffect(() => {
    postEventRef.current = postEvent;
  }, [postEvent]);

  // Built in an effect, not during render: the constructor attaches focus/blur
  // listeners and starts the reporting interval, and those must be undone when
  // the view goes away.
  const [eventReporter, setEventReporter] = useState(null);
  useEffect(() => {
    const reporter = new EventReporterSingleton(async (event_type, value) => {
      const json = await postEventRef.current(event_type, value);
      if (event_type === "answered_problem" && json && json.gamestate) {
        setGamestate(json["gamestate"]);
        setProblem(json["problem"]);
        setVideo(json["video"]);
      }
    }, interval);
    setEventReporter(reporter);
    return () => reporter.tearDown();
  }, [interval]);

  // The dev fast-forward: auto-answer, auto-watch, reload. It posts events, so
  // it belongs in an effect rather than mid-render. Ships false in conf.json.
  const solved = gamestate ? gamestate.solved : null;
  const target = gamestate ? gamestate.target : null;
  const quickplayVideoId = gamestate ? gamestate.video_id : null;
  const correctAnswer = problem ? problem.answer : null;
  useEffect(() => {
    if (!conf.debug_quickplay || solved == null || correctAnswer == null) {
      return;
    }
    let cancelled = false;
    const advance = async () => {
      const post = postEventRef.current;
      if (solved >= target) {
        if ((await post("watching_video", 5000)) == null || cancelled) return;
        if ((await post("done_watching_video", quickplayVideoId)) == null) {
          return;
        }
      } else {
        if ((await post("working_on_problem", 1000)) == null || cancelled) {
          return;
        }
        if ((await post("answered_problem", correctAnswer)) == null) return;
      }
      if (!cancelled) window.location.pathname = "play";
    };
    advance();
    return () => {
      cancelled = true;
    };
  }, [solved, target, quickplayVideoId, correctAnswer]);

  if (!gamestate || !problem || !eventReporter) {
    return <div className="content-loading"></div>;
  }

  if (gamestate.solved >= gamestate.target) {
    // debug_quickplay drives the loop from the effect above; render nothing.
    if (conf.debug_quickplay) {
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
      return null;
    } else {
      const closeReportModal = () => {
        setShowReportModal(false);
        setReportExplanation("");
        setReportError("");
      };

      const handleReportSubmit = (pin) => {
        setReportError("");
        if (!user.pin || user.pin.length < 4) {
          setReportError("Set a PIN in settings first.");
          return;
        }
        if (pin !== user.pin) {
          setReportError("Incorrect PIN");
          return;
        }
        setReportSubmitting(true);
        const value = JSON.stringify({
          problem_id: gamestate.problem_id,
          explanation:
            reportExplanation.trim().slice(0, REPORT_EXPLANATION_MAX_LENGTH) ||
            "",
        });
        postEvent("bad_problem_user", value)
          .then((json) => {
            if (json && json.gamestate) {
              setGamestate(json.gamestate);
              setProblem(json.problem);
              setVideo(json.video);
            }
            closeReportModal();
          })
          .finally(() => setReportSubmitting(false));
      };

      return (
        <>
          <ProblemView
            gamestate={gamestate}
            latex={latex}
            eventReporter={eventReporter}
            interval={interval}
          />
          <button
            type="button"
            className="report-problem-link"
            onClick={() => {
              setShowReportModal(true);
              setReportError("");
              setReportExplanation("");
            }}
          >
            Report problem
          </button>
          {showReportModal && (
            <PinConfirmModal
              title="Report problem"
              copy="Report if this problem is unsuitable or doesn't accept the correct answer. Your PIN is required."
              pinLabel="PIN"
              confirmLabel="Submit"
              submittingLabel="Submitting…"
              submitting={reportSubmitting}
              error={reportError}
              onConfirm={handleReportSubmit}
              onCancel={closeReportModal}
            >
              <div className="report-explanation">
                <label htmlFor="report-explanation">
                  Why are you reporting this problem? (optional)
                </label>
                <textarea
                  id="report-explanation"
                  value={reportExplanation}
                  onChange={(e) =>
                    setReportExplanation(
                      e.target.value.slice(0, REPORT_EXPLANATION_MAX_LENGTH)
                    )
                  }
                  maxLength={REPORT_EXPLANATION_MAX_LENGTH}
                  rows={3}
                  placeholder="e.g. Wrong answer was marked correct"
                />
                <span className="report-char-count">
                  {reportExplanation.length}/{REPORT_EXPLANATION_MAX_LENGTH}
                </span>
              </div>
            </PinConfirmModal>
          )}
        </>
      );
    }
  }
};

export { PlayView };
