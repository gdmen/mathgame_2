import Axios from "axios";
import React from "react";
import ReactPlayer from "react-player"

import {
    GetBaseUrl
} from "../common/util.js";

const ModelEndpoint = "/videos";

class BaseVideo extends React.Component {
    constructor(props) {
        super(props);
        this.state = {
            model: {
                id: this.props.id,
                title: "",
                local_file_name: "",
                enabled: false
            },
            error: null,
            isLoaded: false,
            currTime: 0,
            player: undefined
        };
    }

    getModel() {
        Axios
            .get(this.props.url + ModelEndpoint + "/" + this.state.model.id)
            .then(resp => {
                console.log(resp.data);
                this.setState((state, props) => ({
                    isLoaded: true,
                    model: {
                        ...state.model,
                        title: resp.data.title,
                        local_file_name: resp.data.local_file_name,
                        enabled: resp.data.enabled
                    }
                }));
            })
            .catch(this.catchError.bind(this));
    }

    postModel(id) {
        Axios
            .post(this.props.url + ModelEndpoint + "/" + id, this.state.model)
            .then(resp => {
                this.setState((state, props) => ({
                    isLoaded: true,
                    model: {
                        ...state.model,
                        title: resp.data.title,
                        local_file_name: resp.data.local_file_name,
                        enabled: resp.data.enabled
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
                <div>
                    <p>id: {model.id}</p>
                    <p>local_file_name: {model.local_file_name}</p>
                    <p>enabled: {model.enabled ? "true" : "false"}</p>
                </div>
                <div>
                    <ReactPlayer
                        ref={p => {
                            if(this.state.player) {
                                return;
                            }
                            this.setState((state, props) => ({
                                player: p
                            }));
                        }}
                        url={GetBaseUrl() + "/" + this.props.video_dir + model.local_file_name}
                        playing
                        controls
                        config={{
                            file: {
                                attributes: {
                                    controlsList: "nodownload",
                                    disablepictureinpicture: "true",
                                    onContextMenu: e => e.preventDefault()
                                }
                            }
                        }}
                        onSeek={() => {
                            let delta = this.state.player.getCurrentTime() - this.state.currTime;
                            if (Math.abs(delta) > 0.01) {
                                this.state.player.seekTo(this.state.currTime);
                            }
                        }}
                        onProgress={({played, playedSeconds, loaded, loadedSeconds}) => {
                            let seeking = this.state.player.player.player.player.seeking
                            if (!seeking) {
                                this.setState((state, props) => ({currTime: playedSeconds}));
                            }
                        }}
                        onEnded={() => {
                        }}
                        onError={() => {
                        }}
                    />
                </div>
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
        const value = name === "enabled" ? t.checked : t.value;
        this.setState((state, props) => ({
            model: {
                ...state.model,
                [name]: value
            }
        }));
    }

    handleSubmit(e) {
        e.preventDefault();
        this.postModel(this.state.model.id);
    }

    renderSuccess() {
        let model = this.state.model;
        return (
            <form onSubmit={this.handleSubmit}>
                <p>{model.id}</p>
                <input type="text" name="title" value={model.title} onChange={this.handleChange} />
                <input type="text" name="local_file_name" value={model.local_file_name} onChange={this.handleChange} />
                <input type="checkbox" name="enabled" checked={model.enabled} value={model.enabled} onChange={this.handleChange} />
                <input type="submit" value="Submit" onChange={this.handleChange} />
            </form>
        );
    }
}

class CreateVideo extends EditVideo {

    componentDidMount() {
        this.setState((state, props) => ({
            isLoaded: true
        }));
    }

    handleSubmit(e) {
        e.preventDefault();
        this.postModel("");
    }
}

export {
    BaseVideo,
    EditVideo,
    CreateVideo
}
