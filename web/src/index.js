import React from "react";
import ReactDOM from "react-dom";
import {
    BrowserRouter,
    Route,
    Switch
} from "react-router-dom";

import "./index.css";

import {
    BaseVideo,
    EditVideo
} from "./api/video.js";

const NotFound = () => (
    <div>
        <h3>404 page not found</h3>
    </div>
)

/*
 * TODO:
 * - react import files
 * - video.js file
 *   - video display (id as input)
 *   - video create form
 *   - video edit form
 */

var conf = require("./conf");
const ApiUrl = conf.api_host + ":" + conf.api_port + "/api/v1";

class VideosView extends React.Component {
    render() {
        return (
            <div>
                <BaseVideo id={1} url={ApiUrl} />
                <hr />
                <BaseVideo id={2} url={ApiUrl} />
                <hr />
                <EditVideo id={1} url={ApiUrl} />
            {/*
                <h1>Add a Video</h1>
                <form action={ApiUrl + "/api/v1/videos"} method="post" encType="multipart/form-data">
                    <input type="text" name="title" />
                    <input type="text" name="youtube_id" />
                    <input type="number" name="start" />
                    <input type="number" name="end" />
                    <input type="checkbox" name="enabled" value="1" checked />
                    <input type="submit" value="Submit" />
                </form>
                */}
            </div>
        );
    }
}

const Main = () => (
    <main>
	<Switch>
        <Route path="/videos" component={VideosView} />
	<Route path="*" component={NotFound} />
	</Switch>
    </main>
)

const App = () => (
    <div>
	<Main />
    </div>
)

ReactDOM.render((
    <BrowserRouter>
	<App />
    </BrowserRouter>
), document.getElementById("root"))
