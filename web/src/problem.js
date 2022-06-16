import React from "react"

import './problem.css'

var ReactFitText = require('react-fittext');

const ProblemView = ({ latex, answer, setAnswer, postAnswer }) => {
  return (
    <div id="problem">
        <ReactFitText compressor={0.75}>
            <div id="problem-display" dangerouslySetInnerHTML={{__html: latex}}></div>
        </ReactFitText>
        <div id="problem-answer" className="input-group">
            <input id="problem-answer-input" className="input-group-field" type="text"
                value={answer}
                onChange={(e) => setAnswer(e.target.value)}
            />
            <div className="input-group-button">
                <input type="submit" className="button" value="answer"
                  onClick={postAnswer}
                />
            </div>
        </div>
    </div>
  )
}

export {
  ProblemView
}
