import React from "react";
import Axios from "axios";

const ModelEndpoint = "/videos";

class BaseVideo extends React.Component {
    constructor(props) {
        super(props);
        this.state = {
            model: {
                id: this.props.id,
                title: "",
                youtube_id: "",
                start: 0,
                end: 9999,
                enabled: false
            },
            error: null,
            isLoaded: false,
        };
    }

    getModel() {
        Axios
            .get(this.props.url + ModelEndpoint + "/" + this.state.model.id)
            .then(resp => {
                console.log(resp.data.enabled);
                this.setState((state, props) => ({
                    isLoaded: true,
                    model: {
                        ...state.model,
                        title: resp.data.title,
                        youtube_id: resp.data.youtube_id,
                        start: resp.data.start,
                        end: resp.data.end,
                        enabled: resp.data.enabled
                    }
                }));
            })
            .catch(this.catchError.bind(this))
    }

    postModel() {
        Axios
            .post(this.props.url + ModelEndpoint + "/" + this.state.model.id, this.state.model)
            .then(resp => {
                this.setState((state, props) => ({
                    isLoaded: true,
                    model: {
                        ...state.model,
                        title: resp.data.title,
                        youtube_id: resp.data.youtube_id,
                        start: resp.data.start,
                        end: resp.data.end,
                        enabled: resp.data.enabled
                    }
                }));
            })
            .catch(this.catchError.bind(this))
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
        this.getModel();
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
                <p>id: {model.id}</p>
                <p>youtube_id: {model.youtube_id}</p>
                <p>start: {model.start}</p>
                <p>end: {model.end}</p>
                <p>enabled: {model.enabled ? "true" : "false"}</p>
            </div>
        );
    }
}

class EditVideo extends BaseVideo {

    constructor(props) {
        super(props);
        this.handleChange = this.handleChange.bind(this);
        this.handleSubmit = this.handleSubmit.bind(this);
    }

    handleChange(e) {
        e.persist();
        const t = e.target;
        const name = t.name;
        let value = t.value;
        if (name === "enabled") {
            value = t.checked;
        }
        if (name === "start" || name === "end") {
            if (!Number(value)) {
                return;
            }
            value = Number(value);
        }
        this.setState((state, props) => ({
            model: {
                ...state.model,
                [name]: value
            }
        }));
    }

    handleSubmit(e) {
        e.preventDefault();
        this.postModel();
    }

    renderSuccess() {
        let model = this.state.model;
        return (
            <form onSubmit={this.handleSubmit}>
                <p>{model.id}</p>
                <input type="text" name="title" value={model.title} onChange={this.handleChange} />
                <input type="text" name="youtube_id" value={model.youtube_id} onChange={this.handleChange} />
                <input type="number" name="start" value={model.start} onChange={this.handleChange} />
                <input type="number" name="end" value={model.end} onChange={this.handleChange} />
                <input type="checkbox" name="enabled" checked={model.enabled} value={model.enabled} onChange={this.handleChange} />
                <input type="submit" value="Submit" onChange={this.handleChange} />
            </form>
        );
    }
}

export {
    BaseVideo,
    EditVideo
}
