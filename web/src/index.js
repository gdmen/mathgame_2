import React, { useCallback, useEffect, useState } from "react";
import ReactDOM from 'react-dom'
import { BrowserRouter, Route, Switch } from 'react-router-dom'

import { Auth0Provider } from "@auth0/auth0-react"
import { useAuth0 } from "@auth0/auth0-react";

import { LoginButton, LogoutButton } from './auth0.js'

import { HomeView } from './home.js'
import { PlayView } from './play.js'
import { CompanionView } from './companion.js'

import './index.scss'

const conf = require('./conf')
const ApiUrl = conf.api_host + ':' + conf.api_port + '/api/v1'

const NotFound = () => (
  <div>
    <h3>404 page not found</h3>
  </div>
)

const Main = ({ token, url, isLoading, isAuthenticated, user, postEvent }) => {
  return (
    <main>
      <Switch>
        <Route exact path="/">
          <HomeView isLoading={isLoading} isAuthenticated={isAuthenticated} user={user}/>
        </Route>
        <Route path="/play">
          {!isLoading && isAuthenticated &&
            <PlayView token={token} url={url} user={user} postEvent={postEvent} interval={conf.event_reporting_interval} />
          }
        </Route>
        <Route path="/companion/:student_id">
          {!isLoading && isAuthenticated &&
            <CompanionView token={token} url={url} user={user} />
          }
        </Route>
        <Route path="*" component={NotFound} />
      </Switch>
    </main>
  )
}

const App = () => {
  const {user, isLoading, isAuthenticated, getAccessTokenSilently} = useAuth0();
  const [token, setToken] = useState(null);
  const [appUser, setAppUser] = useState(null);

  const genPostEventFcn = useCallback(() => {
    return async function(event_type, value) {
      try {
        const settings = {
            method: 'POST',
            headers: {
              'Accept': 'application/json',
              'Content-Type': 'application/json',
              'Authorization': 'Bearer ' + token,
            },
            body: JSON.stringify({
              'event_type': event_type,
              'value': String(value),
            }),
        };
        console.log("reporting " + event_type + ":" + String(value));
        const req = await fetch(ApiUrl+ "/events", settings);
        const json = await req.json();
        return json;
      } catch (e) {
        console.log(e.message);
      }
    };
  }, [token]);

  useEffect(() => {
    const getToken = async () => {
      try {
        setToken(await getAccessTokenSilently());
      } catch (e) {
        console.log(e.message);
      }
    };

    getToken();
  }, [getAccessTokenSilently]);

  useEffect(() => {
    const getAppUser = async () => {
      try {
        if (token == null) {
          return;
        }
        const settings = {
            method: 'POST',
            headers: {
                'Accept': 'application/json',
                'Content-Type': 'application/json',
                'Authorization': 'Bearer ' + token,
            },
            body: JSON.stringify({
              auth0_id: user.sub,
              email: user.email,
              username: user.name,
            })
        };
        const req = await fetch(ApiUrl + "/users", settings);
        const json = await req.json();
        setAppUser(json);
        genPostEventFcn("logged_in", "");
      } catch (e) {
        console.log(e.message);
      }
    };

    getAppUser();
  }, [token, user, genPostEventFcn]);

  return (
    <div>
      <div id="main-menu" className="clearfix">
        <a href="/"><h3>The Math Game</h3></a>

        <ul className="menu">
          <li>{user ? user.username : ""}</li>
          <li>
            {isAuthenticated ? <LogoutButton /> : <LoginButton />}
          </li>
        </ul>
      </div>

      <div id="content">
            <Main token={token} url={ApiUrl} isLoading={isLoading} isAuthenticated={isAuthenticated} user={appUser} postEvent={genPostEventFcn()}/>
      </div>
    </div>
  )
}

ReactDOM.render(
  <BrowserRouter>
    <Auth0Provider
      audience={conf.auth0_audience}
      clientId={conf.auth0_clientId}
      domain={conf.auth0_domain}
      redirectUri={window.location.origin}
    >
    <App />
  </Auth0Provider>
  </BrowserRouter>,
  document.getElementById('react')
)
