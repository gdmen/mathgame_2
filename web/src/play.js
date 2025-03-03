import katex from "katex";
import React, { useCallback, useEffect, useState } from "react";

import "katex/dist/katex.min.css";

import { ProblemView, IsWordProblem } from "./problem.js";
import { VideoView } from "./video.js";
import { ClearSessionPin } from "./pin.js";

const conf = require("./conf");

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

  const preprocessExpression = (expression) => {
    function replacer(match, offset, string) {
      return match.replace(/\s/g, " }\\text{");
    }
    return expression.replace(/\\text\{[^\}]+\}/g, replacer);
  };

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
      setLatex(
        katex.renderToString(preprocessExpression(json.expression), {
          displayMode: false,
        })
      );
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

  useEffect(() => {
    getGamestate();
  }, [getGamestate]);

  useEffect(() => {
    getProblem();
  }, [getProblem]);

  useEffect(() => {
    getVideo();
  }, [getVideo]);

  const postAnswer = async (answer) => {
    setGamestate(await postEvent("answered_problem", answer));
  };

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
    } else {
      return (
        <VideoView video={video} postEvent={postEvent} interval={interval} />
      );
    }
  } else {
    if (conf.debug_quickplay) {
      postEvent("working_on_problem", 1000).then(() => {
        postAnswer(problem.answer, gamestate.problem_id).then(() => {
          window.location.pathname = "play";
        });
      });
    } else {
      return (
        <ProblemView
          gamestate={gamestate}
          latex={latex}
          isWordProblem={IsWordProblem(problem)}
          postAnswer={postAnswer}
          postEvent={postEvent}
          interval={interval}
        />
      );
    }
  }
};

export { PlayView };
