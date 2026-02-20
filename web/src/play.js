import katex from "katex";
import React, { useCallback, useEffect, useState } from "react";
import PinInput from "react-pin-input";

import "katex/dist/katex.min.css";

import { ProblemView, PreprocessExpression } from "./problem.js";
import { VideoView } from "./video.js";
import { ClearSessionPin } from "./pin.js";

import "./play.scss";

const REPORT_EXPLANATION_MAX_LENGTH = 500;

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
  const [showReportModal, setShowReportModal] = useState(false);
  const [reportPin, setReportPin] = useState("");
  const [reportExplanation, setReportExplanation] = useState("");
  const [reportError, setReportError] = useState("");
  const [reportSubmitting, setReportSubmitting] = useState(false);

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
        const text = await req.text();
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

  const eventReporter = new EventReporterSingleton(
    async (event_type, value) => {
      let json = await postEvent(event_type, value);
      if (event_type == "answered_problem" && json && json.gamestate) {
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
      postEvent("watching_video", 5000).then((json) => {
        if (json == null) return;
        postEvent("done_watching_video", gamestate.video_id).then(
          (doneJson) => {
            if (doneJson != null) window.location.pathname = "play";
          }
        );
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
      postEvent("working_on_problem", 1000).then((json) => {
        if (json == null) return;
        postEvent("answered_problem", problem.answer).then((answeredJson) => {
          if (answeredJson != null) window.location.pathname = "play";
        });
      });
      return null;
    } else {
      const handleReportSubmit = () => {
        setReportError("");
        if (!user.pin || user.pin.length < 4) {
          setReportError("Set a PIN in settings first.");
          return;
        }
        if (reportPin.length !== 4 || reportPin !== user.pin) {
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
            setShowReportModal(false);
            setReportPin("");
            setReportExplanation("");
            setReportError("");
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
              setReportPin("");
              setReportExplanation("");
            }}
          >
            Report problem
          </button>
          {showReportModal && (
            <div
              className="report-modal-overlay"
              onClick={() => !reportSubmitting && setShowReportModal(false)}
            >
              <div
                className="report-modal"
                onClick={(e) => e.stopPropagation()}
              >
                <h4>Report problem</h4>
                <p className="report-modal-copy">
                  Report if this problem is unsuitable or doesn&apos;t accept
                  the correct answer. Your PIN is required.
                </p>
                <div className="report-modal-pin">
                  <label htmlFor="report-pin">PIN</label>
                  <PinInput
                    id="report-pin"
                    length={4}
                    type="numeric"
                    inputMode="number"
                    value={reportPin}
                    onChange={(value) => setReportPin(value)}
                    inputStyle={{ borderRadius: "0.25em" }}
                  />
                </div>
                <div className="report-modal-explanation">
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
                {reportError && (
                  <p className="report-modal-error">{reportError}</p>
                )}
                <div className="report-modal-actions">
                  <button
                    type="button"
                    onClick={handleReportSubmit}
                    disabled={reportSubmitting}
                  >
                    {reportSubmitting ? "Submitting…" : "Submit"}
                  </button>
                  <button
                    type="button"
                    onClick={() =>
                      !reportSubmitting && setShowReportModal(false)
                    }
                    disabled={reportSubmitting}
                  >
                    Cancel
                  </button>
                </div>
              </div>
            </div>
          )}
        </>
      );
    }
  }
};

export { PlayView };
