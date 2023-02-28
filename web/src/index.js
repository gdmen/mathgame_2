import React, { useCallback, useEffect, useState } from "react";
import ReactDOM from 'react-dom'
import { BrowserRouter, Route, Switch } from 'react-router-dom'

import { Auth0Provider } from "@auth0/auth0-react"
import { useAuth0 } from "@auth0/auth0-react";

import { LoginButton, LogoutButton } from './auth0.js'

import { HomeView } from './home.js'
import { SetupView } from './setup.js'
import { SettingsView } from './settings.js'
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

const MainView = ({ token, url, isLoading, isAuthenticated, user, settings, postEvent }) => {
  if (isLoading || (isAuthenticated && settings == null)) {
    return (
      <div className="content-loading"></div>
    )
  }
  // TODO: setup if: (pin not set OR no operations set OR no videos set)
  else if (settings != null && user.pin === "") {
    return <SetupView token={token} url={url} user={user} settings={settings}/>
  }
  else {
    // TODO: in the following switch, if not auth'd, redirect to landing page
    return (
      <main>
        <Switch>
          <Route exact path="/">
            <HomeView isLoading={isLoading} isAuthenticated={isAuthenticated} user={user} settings={settings}/>
          </Route>
          <Route exact path="/settings">
            {!isLoading && isAuthenticated &&
              <SettingsView token={token} url={url} user={user} settings={settings}/>
            }
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
}

const AppView = () => {
  const {user, isLoading, isAuthenticated, getAccessTokenSilently} = useAuth0();
  const [token, setToken] = useState(null);
  const [appUser, setAppUser] = useState(null);
  const [settings, setSettings] = useState(null);

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
    const getSettings = async () => {
      try {
        if (token == null || appUser == null) {
          return;
        }
        const settings = {
            method: 'GET',
            headers: {
                'Accept': 'application/json',
                'Content-Type': 'application/json',
                'Authorization': 'Bearer ' + token,
            }
        };
        const req = await fetch(ApiUrl + "/settings/" + appUser.id, settings);
        const json = await req.json();
        setSettings(json);
      } catch (e) {
        console.log(e.message);
      }
    };

    getSettings();
  }, [token, appUser]);

  useEffect(() => {
    const getAppUser = async () => {
      try {
        if (token == null || user == null || genPostEventFcn == null) {
          return;
        }
        const settings = {
            method: 'GET',
            headers: {
                'Accept': 'application/json',
                'Content-Type': 'application/json',
                'Authorization': 'Bearer ' + token,
            },
        };
        const req = await fetch(ApiUrl + "/users/" + encodeURIComponent(user.sub), settings);
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
            <MainView token={token} url={ApiUrl} isLoading={isLoading} isAuthenticated={isAuthenticated} user={appUser} settings={settings} postEvent={genPostEventFcn()}/>
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
    <AppView />
  </Auth0Provider>
  </BrowserRouter>,
  document.getElementById('react')
)
