import React, { useEffect, useState } from "react";
import ReactDOM from 'react-dom'
import { BrowserRouter, Route, Switch } from 'react-router-dom'

import { Auth0Provider } from "@auth0/auth0-react"
import { useAuth0 } from "@auth0/auth0-react";

import { LoginButton } from './auth0.js'

import { HomeView } from './home.js'
import { PlayView } from './play.js'

import 'foundation-sites/dist/css/foundation.css'

const conf = require('./conf')
const ApiUrl = conf.api_host + ':' + conf.api_port + '/api/v1'

const NotFound = () => (
  <div>
    <h3>404 page not found</h3>
  </div>
)

const Main = ({ token, url, isLoading, isAuthenticated, user }) => {
  return (
    <main>
      <Switch>
        <Route exact path="/">
          <HomeView isLoading={isLoading} isAuthenticated={isAuthenticated} user={user}/>
        </Route>
        <Route path="/play">
          {!isLoading && isAuthenticated &&
            <PlayView token={token} url={url} user={user}/>
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
      } catch (e) {
        console.log(e.message);
      }
    };

    getAppUser();
  }, [token, user]);

  return (
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
              <a href="/play">play</a>
            </li>
          </ul>
        </div>
      </div>

      <div className="grid-container full">
        <div className="grid-x grid-margin-x align-center">
          <div className="cell small-11 medium-8 large-7">
            <Main token={token} url={ApiUrl} isLoading={isLoading} isAuthenticated={isAuthenticated} user={appUser}/>
          </div>
        </div>
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
