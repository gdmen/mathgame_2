import Axios from "axios";
import React from "react";

let mathjs = require('mathjs');

const ModelEndpoint = "/problems";

class BaseProblem extends React.Component {
    constructor(props) {
        super(props);
        this.state = {
            options: {
                operations: ["+", "-"],
                fractions: false,
                negatives: false,
                target_difficulty: 6
            },
            model: {
                id: 0,
                expression: "",
                answer: "",
                difficulty: 0
            },
            error: null,
            isLoaded: false
        };
    }

    createModel() {
        Axios
            .post(this.props.url + ModelEndpoint, this.state.options)
            .then(resp => {
                this.setState((state, props) => ({
                    isLoaded: true,
                    model: {
                        id: resp.data.id,
                        expression: resp.data.expression,
                        answer: resp.data.answer,
                        difficulty: resp.data.difficulty
                    }
                }));
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
        this.createModel();
    }

    render() {
        if (this.state.error) {
            return <div>Error: {this.state.error.message}</div>;
        } else if (!this.state.isLoaded) {
            return <div>Loading...</div>;
        } else {
            return this.renderSuccess();
        }
    }

    renderSuccess() {
        let model = this.state.model;
        return (
            <div>
                <div>
                    <p>id: {model.id}</p>
                    <p>expression: {model.expression}</p>
                    <p>answer: {model.answer} ({mathjs.evaluate(model.expression)})</p>
                    <p>difficulty: {model.difficulty}</p>
                </div>
            </div>
        );
    }
}

export {
    BaseProblem
}
