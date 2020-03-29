import React from "react";
import ReactDOM from "react-dom";
import { BrowserRouter, Route, Switch } from "react-router-dom";
import Axios from "axios";

import "./index.css";

const NotFound = () => (
	<div>
		<h3>404 page not found</h3>
	</div>
)

var conf = require("./conf");
const ApiUrl = conf.api_host + ":" + conf.api_port;

class VideosView extends React.Component {
	render() {
		return (
				<div>
                                        <h1>Add a Video</h1>
					<form action={ApiUrl + "/api/v1/videos"} method="post" encType="multipart/form-data">
                                        <input type="text" name="title" />
                                        <input type="text" name="youtube_id" />
                                        <input type="number" name="start" />
                                        <input type="number" name="end" />
                                        <input type="checkbox" name="enabled" value="1" checked />
					<input type="submit" value="Submit" />
					</form>
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
