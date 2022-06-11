import katex from 'katex';
import React, { useEffect, useState } from "react";

import "katex/dist/katex.min.css"
import './play.css'

var ReactFitText = require('react-fittext');

const PlayView = ({ token, url, user }) => {
  const [gamestate, setGamestate] = useState(null);
  const [problem, setProblem] = useState(null);

  useEffect(() => {
    const getGamestate = async () => {
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
    };

    getGamestate();
  }, [token, url, user]);

  useEffect(() => {
    const getProblem = async () => {
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
      } catch (e) {
        console.log(e.message);
      }
    };

    getProblem();
  }, [token, url, gamestate]);

  if (!gamestate || !problem) {
    return (
      <div>loading</div>
    )
  }
  var latex = katex.renderToString(problem.expression);
  return (
    <div id="problem">
        <ReactFitText compressor={0.75}>
            <div id="problem-display" dangerouslySetInnerHTML={{__html: latex}}></div>
        </ReactFitText>
        <div id="problem-answer" className="input-group">
            <input id="problem-answer-input" className="input-group-field" type="text" />
            <div className="input-group-button">
                <input type="submit" className="button" value="answer" />
            </div>
        </div>
    </div>
  )
}

export {
  PlayView
}
