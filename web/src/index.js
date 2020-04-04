import React from "react";
import ReactDOM from "react-dom";
import {
    BrowserRouter,
    Route,
    Switch
} from "react-router-dom";

import {
    BaseVideo,
    CreateVideo,
    EditVideo
} from "./api/video.js";

import "./index.css";

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

class AdminVideosView extends React.Component {
    render() {
        return (
            <div>
                <CreateVideo url={ApiUrl} />
                <hr />
                <BaseVideo id={1} url={ApiUrl} video_dir={conf.video_dir}/>
                <hr />
                <BaseVideo id={2} url={ApiUrl} video_dir={conf.video_dir}/>
                <hr />
                <EditVideo id={1} url={ApiUrl} />
            </div>
        );
    }
}

const Main = () => (
    <main>
	<Switch>
        <Route path="/admin/videos" component={AdminVideosView} />
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
