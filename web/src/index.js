import Axios from "axios";

import React, { useEffect, useState } from "react";
import ReactDOM from 'react-dom'
import { BrowserRouter, Route, Switch } from 'react-router-dom'

import { Auth0Provider } from "@auth0/auth0-react"
import { useAuth0 } from "@auth0/auth0-react";

import { LoginButton } from './auth0.js'
import { BaseVideo, CreateVideo, EditVideo } from './api/video.js'

import { BaseProblem } from './api/problem.js'

import 'foundation-sites/dist/css/foundation.css'
import './index.css'

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

var conf = require('./conf')
const ApiUrl = conf.api_host + ':' + conf.api_port + '/api/v1'

const IndexView = () => {
  const { user, isLoading, isAuthenticated, getAccessTokenSilently} = useAuth0();
  const [accessToken, setAccessToken] = useState(null);
  useEffect(() => {
    const getAccessToken = async () => {
      try {
        setAccessToken(await getAccessTokenSilently());
      } catch (e) {
        console.log(e.message);
      }
    };

    getAccessToken();
  }, [getAccessTokenSilently]);

  if (isLoading) {
    return <div>loading... </div>;
  }

  var config = { headers: { Authorization: `Bearer ` + accessToken } };
  Axios
      .post(ApiUrl + "/users", {auth0_id: user.sub, email: user.email, username: user.name}, config)

  return (
    <div>
      hello home page
    </div>
  )
}

class AdminVideosView extends React.Component {
  render() {
    return (
      <div>
        <CreateVideo url={ApiUrl} />
        <hr />
        <BaseVideo
          id={1}
          url={ApiUrl}
          public_video_dir={conf.public_video_dir}
        />
        <hr />
        <BaseVideo
          id={2}
          url={ApiUrl}
          public_video_dir={conf.public_video_dir}
        />
        <hr />
        <EditVideo id={1} url={ApiUrl} />
      </div>
    )
  }
}

const ProblemView = () => {
  const { user, isLoading, isAuthenticated, getAccessTokenSilently} = useAuth0();
  const [accessToken, setAccessToken] = useState(null);
  useEffect(() => {
    const getAccessToken = async () => {
      try {
        setAccessToken(await getAccessTokenSilently());
      } catch (e) {
        console.log(e.message);
      }
    };

    getAccessToken();
  }, [getAccessTokenSilently]);

  if (isLoading) {
    return <div>loading... </div>;
  }

  return (
    <div>
      <BaseProblem url={ApiUrl} accessToken={accessToken} auth0Id={user.sub} />
    </div>
  )
}

const Main = () => (
  <main>
    <Switch>
      <Route path="/admin/videos" component={AdminVideosView} />
      <Route path="/problem" component={ProblemView} />
      <Route path="" component={IndexView} />
      <Route path="*" component={NotFound} />
    </Switch>
  </main>
)

const App = () => (
  <div>
    <div className="top-bar">
      <div className="top-bar-left">
        <ul className="menu">
          <li className="menu-text">The Math Game</li>
        </ul>
      </div>

      <div className="top-bar-right">
        <ul className="menu">
          <li>
            <LoginButton />
          </li>
          <li>
            <a href="/">/</a>
          </li>
          <li>
            <a href="/problem">problem</a>
          </li>
        </ul>
      </div>
    </div>

    <div className="grid-container full">
      <div className="grid-x grid-margin-x align-center">
        <div className="cell small-11 medium-8 large-7">
          <Main />
        </div>
      </div>
    </div>
  </div>
)

ReactDOM.render(
  <BrowserRouter>
    <Auth0Provider
      domain="compteam.auth0.com"
      clientId="IJt7c4yK6NhRGpIvmBYxLtWCCQbtCekZ"
      redirectUri={window.location.origin}
      //audience="https://compteam.auth0.com/api/v2/"
      audience="mathgame"
      //scope="test:access"
      //scope="read:current_user update:current_user_metadata test:access"
    >
    <App />
  </Auth0Provider>
  </BrowserRouter>,
  document.getElementById('react')
)
