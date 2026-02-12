import React, { useCallback, useEffect, useState } from "react";
import ReactDOM from "react-dom";
import { BrowserRouter, Route, Switch } from "react-router-dom";

import { Auth0Provider } from "@auth0/auth0-react";
import { useAuth0 } from "@auth0/auth0-react";

import { LoginButton, LogoutButton } from "./auth0.js";

import { HomeView } from "./home.js";
import { SetupView } from "./setup.js";
import { PinView, ClearSessionPin } from "./pin.js";
import { SettingsView } from "./settings.js";
import { PlayView } from "./play.js";
import { CompanionView } from "./companion.js";

import "./index.scss";

const conf = require("./conf");
const ApiUrl = conf.api_host + ":" + conf.api_port + "/api/v1";

const NotFound = () => {
  ClearSessionPin();
  return (
    <div>
      <h3>404 page not found</h3>
    </div>
  );
};

const MainView = ({
  token,
  apiUrl,
  isLoading,
  isAuthenticated,
  user,
  settings,
  numEnabledVideos,
  refreshPageLoadData,
  postEvent,
}) => {
  if (isLoading || (isAuthenticated && settings == null)) {
    return <div className="content-loading"></div>;
  } else if (settings != null && (user.pin === "" || numEnabledVideos < 3)) {
    return (
      <SetupView
        token={token}
        apiUrl={apiUrl}
        user={user}
        settings={settings}
        numEnabledVideos={numEnabledVideos}
        refreshPageLoadData={refreshPageLoadData}
      />
    );
  } else {
    // TODO: in the following switch, if not auth'd, redirect to landing page
    return (
      <main>
        <Switch>
          <Route exact path="/">
            <HomeView
              isLoading={isLoading}
              isAuthenticated={isAuthenticated}
              user={user}
              settings={settings}
            />
          </Route>
          <Route exact path="/pin/:redirect_pathname">
            {!isLoading && isAuthenticated && <PinView user={user} />}
          </Route>
          <Route exact path="/play">
            {!isLoading && isAuthenticated && (
              <PlayView
                token={token}
                apiUrl={apiUrl}
                user={user}
                postEvent={postEvent}
                interval={conf.event_reporting_interval}
              />
            )}
          </Route>
          <Route exact path="/settings">
            {!isLoading && isAuthenticated && (
              <SettingsView
                token={token}
                apiUrl={apiUrl}
                user={user}
                settings={settings}
              />
            )}
          </Route>
          <Route exact path="/companion/:student_id">
            {!isLoading && isAuthenticated && (
              <CompanionView token={token} apiUrl={apiUrl} user={user} />
            )}
          </Route>
          <Route path="*" component={NotFound} />
        </Switch>
      </main>
    );
  }
};

const AppView = () => {
  const { user, isLoading, isAuthenticated, getAccessTokenSilently } =
    useAuth0();
  const [token, setToken] = useState(null);
  const [appUser, setAppUser] = useState(null);
  const [settings, setSettings] = useState(null);
  const [numEnabledVideos, setNumEnabledVideos] = useState(null);

  const genPostEventFcn = useCallback(() => {
    return async function (event_type, value) {
      try {
        const reqParams = {
          method: "POST",
          headers: {
            Accept: "application/json",
            "Content-Type": "application/json",
            Authorization: "Bearer " + token,
          },
          body: JSON.stringify({
            event_type: event_type,
            value: String(value),
          }),
        };
        console.log("reporting " + event_type + ":" + String(value));
        const req = await fetch(ApiUrl + "/events", reqParams);
        const text = await req.text();
        if (!text || text.trim() === "") {
          console.log("Events API returned empty body");
          return null;
        }
        try {
          return JSON.parse(text);
        } catch (parseErr) {
          console.log("Events API invalid JSON: " + parseErr.message);
          return null;
        }
      } catch (e) {
        console.log(e.message);
        return null;
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
    if (isAuthenticated) {
      getToken();
    }
  }, [isAuthenticated, getAccessTokenSilently]);

  const refreshPageLoadData = useCallback(async () => {
    try {
      if (token == null || user == null) {
        return;
      }
      var reqParams = {
        method: "GET",
        headers: {
          Accept: "application/json",
          "Content-Type": "application/json",
          Authorization: "Bearer " + token,
        },
      };
      var req = await fetch(
        ApiUrl + "/pageload/" + encodeURIComponent(user.sub),
        reqParams
      );
      if (req.status === 404) {
        reqParams.method = "POST";
        reqParams.body = JSON.stringify({
          auth0_id: user.sub,
          email: user.email,
          username: user.name,
        });
        await fetch(ApiUrl + "/users", reqParams);
        reqParams.method = "GET";
        reqParams.body = null;
        req = await fetch(
          ApiUrl + "/pageload/" + encodeURIComponent(user.sub),
          reqParams
        );
      }
      const json = await req.json();
      setAppUser(json["user"]);
      setSettings(json["settings"]);
      setNumEnabledVideos(parseInt(json["num_videos_enabled"]));
    } catch (e) {
      console.log(e.message);
    }
  }, [token, user]);

  useEffect(() => {
    refreshPageLoadData();
  }, [refreshPageLoadData]);

  return (
    <div id="react-body">
      <div id="main-menu" className="clearfix">
        <a href="/">
          <h3>Mikey's Math Game</h3>
        </a>

        <ul className="menu">
          <li>{user ? user.username : ""}</li>
          <li>
            {isAuthenticated ? (
              <button onClick={() => (window.location.pathname = "settings")}>
                Adults
              </button>
            ) : (
              <></>
            )}
            {isAuthenticated ? <LogoutButton /> : <LoginButton />}
          </li>
        </ul>
      </div>

      <div id="content">
        <MainView
          token={token}
          apiUrl={ApiUrl}
          isLoading={isLoading}
          isAuthenticated={isAuthenticated}
          user={appUser}
          settings={settings}
          numEnabledVideos={numEnabledVideos}
          refreshPageLoadData={refreshPageLoadData}
          postEvent={genPostEventFcn()}
        />
      </div>

      <div id="footer">
        {window.location.pathname !== "/play" && (
          <>
            <a
              href="https://forms.gle/r8uUSwyAoNivga3TA"
              target="_blank"
              rel="noopener noreferrer"
            >
              report an issue
            </a>
            <span className="separator">|</span>
            <a
              href="https://github.com/gdmen/mathgame_2"
              target="_blank"
              rel="noopener noreferrer"
            >
              source code
            </a>
          </>
        )}
      </div>
    </div>
  );
};

ReactDOM.render(
  <BrowserRouter>
    <Auth0Provider
      audience={conf.auth0_audience}
      clientId={conf.auth0_clientId}
      domain={conf.auth0_domain}
      redirectUri={window.location.origin}
      cacheLocation="localstorage"
    >
      <AppView />
    </Auth0Provider>
  </BrowserRouter>,
  document.getElementById("react")
);
