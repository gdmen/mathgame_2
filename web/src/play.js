import katex from 'katex';
import React, { useCallback, useEffect, useState } from "react";

import "katex/dist/katex.min.css"

import { ProblemView } from './problem.js'
import { VideoView } from './video.js'

const PlayView = ({ token, url, user, postEvent, interval }) => {
  const [gamestate, setGamestate] = useState(null);
  const [problem, setProblem] = useState(null);
  const [latex, setLatex] = useState(null);
  const [video, setVideo] = useState(null);

  const getGamestate = useCallback(async () => {
    try {
      if (token == null || url == null || user == null) {
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
      const req = await fetch(url+ "/gamestates/" + user.id, settings);
      const json = await req.json();
      setGamestate(json);
    } catch (e) {
      console.log(e.message);
    }
  }, [token, url, user]);

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
      setLatex(katex.renderToString(json.expression));
    } catch (e) {
      console.log(e.message);
    }
  }, [token, url, gamestate]);

  const getVideo = useCallback(async () => {
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
      const req = await fetch(url+ "/videos/" + gamestate.video_id, settings);
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
    return <div className="content-loading">loading</div>
  }

  if (gamestate.solved >= gamestate.target) {
    return <VideoView video={video} postEvent={postEvent} interval={interval} />
  }
  return <ProblemView gamestate={gamestate} latex={latex} postAnswer={postAnswer} postEvent={postEvent} interval={interval} />
}

export {
  PlayView
}
