import katex from 'katex';
import React, { useCallback, useEffect, useState } from "react";

import "katex/dist/katex.min.css"

import { ProblemView } from './problem.js'

const PlayView = ({ token, url, user, postEvent}) => {
  const [gamestate, setGamestate] = useState(null);
  const [problem, setProblem] = useState(null);
  const [latex, setLatex] = useState(null);
  const [answer, setAnswer] = useState(null);

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

  useEffect(() => {
    getGamestate();
  }, [getGamestate]);

  useEffect(() => {
    getProblem();
  }, [getProblem]);

  useEffect(() => {
    postEvent("displayed_problem", "");
  }, [postEvent, problem]);

  const postAnswer = async () => {
    setGamestate(await postEvent("answered_problem", answer));
  };

  if (!gamestate || !problem) {
    return (
      <div>loading</div>
    )
  }
  if (answer == null) {
    setAnswer("");
  }
  if (gamestate.num_solved >= gamestate.num_target) {
    return <div>VIDEO</div>
  }
  return <ProblemView latex={latex} answer={answer} setAnswer={setAnswer} postAnswer={postAnswer}/>
}

export {
  PlayView
}
