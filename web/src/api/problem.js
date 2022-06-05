import Axios from "axios";
import katex from 'katex';
import React from "react";

import "katex/dist/katex.min.css"

var ReactFitText = require('react-fittext');

const ModelEndpoint = "/problems/";

class BaseProblem extends React.Component {
    constructor(props) {
        super(props);
        this.state = {
            config: { headers: { Authorization: `Bearer ` + this.props.accessToken } },
            problem_id: 0,
            user_id: 0,
            model: {
                id: 0,
                expression: "",
                answer: "",
                difficulty: 0
            },
            latex: "",
            error: null,
            isLoaded: false
        };
    }

    getModel() {
        Axios
            .get(this.props.url + "/gamestates/" + this.state.user_id, this.state.config)
            .then(resp => {
                this.setState((state, props) => ({
                    problem_id: resp.data.problem_id
                }));
                Axios
                    .get(this.props.url + ModelEndpoint + this.state.problem_id, this.state.config)
                    .then(resp => {
                        this.setState((state, props) => ({
                            isLoaded: true,
                            model: {
                                id: resp.data.id,
                                expression: resp.data.expression,
                                answer: resp.data.answer,
                                difficulty: resp.data.difficulty
                            },
                            latex: katex.renderToString(resp.data.expression, {
                                throwOnError: false
                            })
                        }));
                    })
                    .catch(this.catchError.bind(this));
            })
            .catch(this.catchError.bind(this));
    }

    catchError(err) {
        if (err.response) {
            // The request was made and the server responded with a status code
            // that falls out of the range of 2xx
        } else if (err.request) {
            // The request was made but no response was received
            // `err.request` is an instance of XMLHttpRequest in the browser and an instance of
            // http.ClientRequest in node.js
        } else {
            // Something happened in setting up the request that triggered an Error
        }
        this.setState((state, props) => ({
            isLoaded: true,
            error: err
        }));
    }

    componentDidMount() {
        Axios
            .get(this.props.url + "/users/" + this.props.auth0Id, this.state.config)
            .then(resp => {
                this.setState((state, props) => ({
                    user_id: resp.data.id
                }));
                this.getModel();
            })
            .catch(this.catchError.bind(this));
    }

    render() {
        if (this.state.error) {
            return <div>Error: {this.state.error.message}</div>;
        } else if (!this.state.isLoaded) {
            return <div id="problem">Loading...</div>;
        } else {
            return this.renderSuccess();
        }
    }

    renderSuccess() {
        return (
            <div id="problem">
                <ReactFitText compressor={0.75}>
                    <div id="problem-display" dangerouslySetInnerHTML={{__html: this.state.latex}}></div>
                </ReactFitText>
                <div id="problem-answer" className="input-group">
                    <input id="problem-answer-input" className="input-group-field" type="text" />
                    <div className="input-group-button">
                        <input type="submit" className="button" value="answer" />
                    </div>
                </div>
            </div>
        );
    }
}

export {
    BaseProblem
}
