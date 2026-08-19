import React, { useCallback, useEffect, useState } from "react";
import ReactDOM from "react-dom";
import {
  BrowserRouter,
  Redirect,
  Route,
  Switch,
  useParams,
} from "react-router-dom";

import { Auth0Provider } from "@auth0/auth0-react";
import { useAuth0 } from "@auth0/auth0-react";

import { apiFetch } from "./api.js";
import { LoginButton, LogoutButton } from "./auth0.js";

import { SetupView, VideosRepairView, useTakeover } from "./setup.js";
import { PinView, usePinSessionPolicy } from "./pin.js";
import { SettingsView } from "./settings.js";
import { PlayView } from "./play.js";
import { ProgressView } from "./progress.js";
import { AdminHomeView } from "./admin_home.js";
import { DifficultyCalibrationView } from "./admin_calibration.js";
import { BitmapMatrixView } from "./admin_bitmap_matrix.js";
import { StyleGuideView } from "./style_guide.js";

// Self-hosted fonts (families declared in styles.scss). Only the weights the
// design actually uses are loaded; adding a weight here means documenting it
// on /style-guide. Caveat is decorative-only (the landing page's annotation).
import "@fontsource/quicksand/600.css";
import "@fontsource/quicksand/700.css";
import "@fontsource/nunito/400.css";
import "@fontsource/nunito/400-italic.css";
import "@fontsource/nunito/600.css";
import "@fontsource/nunito/700.css";
import "@fontsource/caveat/700.css";

import "./index.scss";

const conf = require("./conf");
const ApiUrl = conf.api_host + "/api/v1";

const SiteName = "Mikey's Math Game";

const useDocumentTitle = (title) => {
  useEffect(() => {
    document.title = title + " · " + SiteName;
  }, [title]);
};

const Titled = ({ title, children }) => {
  useDocumentTitle(title);
  return <>{children}</>;
};

// Several routes render this, so the title comes with it rather than from them.
const NotFound = () => {
  useDocumentTitle("Page not found");
  return (
    <div className="not-found">
      <h1>Oops! We can't find that page.</h1>
      <a href="/">Back to the home page</a>
    </div>
  );
};

// The landing page is a separate static document, so getting there needs a real
// navigation rather than a client-side <Redirect>.
const ToLanding = () => {
  useEffect(() => {
    // Guard against a reload loop: if "/" is already what served this bundle,
    // navigating there again would just re-serve it forever. That should not
    // happen (prod swaps in the static page, dev has setupProxy.js), so the
    // guard is a backstop rather than a path we expect to take.
    if (window.location.pathname !== "/") {
      window.location.replace("/");
    }
  }, []);
  return <div className="content-loading"></div>;
};

// Protected routes render nothing for an unauthenticated visitor, leaving a
// blank screen; send them to the landing page (which offers Login/Signup).
const RequireAuth = ({ isAuthenticated, children }) =>
  isAuthenticated ? children : <ToLanding />;

// PinView proves the code; this route decides where proving it lands you. The
// redirect lives here rather than in the component so PinView stays usable by
// callers that aren't routes at all (the videos repair page's inline gate).
// It also supplies the page: this is the only surface where PinView is the
// whole screen rather than a section of one, so it is the only place that has
// to clear the menu band itself. The wizard step and the videos repair page
// both sit inside pages that already carry that padding.
const PinGateRoute = ({ user }) => {
  const { redirect_pathname } = useParams();
  return (
    <div className="pin-page">
      <PinView
        verifyAgainst={user.pin}
        onValid={() => {
          window.location.pathname = decodeURIComponent(redirect_pathname);
        }}
      />
    </div>
  );
};

// The marketing page at "/" is static HTML outside the React app, so it cannot
// call Auth0 itself. Its CTAs point here instead: this route exists only to
// hand off to Auth0, and to bounce an already-signed-in visitor into play.
const LoginView = () => {
  const { isLoading, isAuthenticated, loginWithRedirect } = useAuth0();
  useEffect(() => {
    if (!isLoading && !isAuthenticated) {
      loginWithRedirect();
    }
  }, [isLoading, isAuthenticated, loginWithRedirect]);
  if (!isLoading && isAuthenticated) {
    return <Redirect to="/play" />;
  }
  return <div className="content-loading"></div>;
};

const MainView = ({
  token,
  apiUrl,
  isLoading,
  isAuthenticated,
  pageLoad,
  refreshPageLoadData,
  postEvent,
}) => {
  const { user, settings } = pageLoad || {};
  // Admin pages bypass the takeovers (an admin can use them without
  // completing setup), and a non-admin who hits one gets the 404 page rather
  // than the setup wizard or any hint the admin surface exists. The /pin
  // gate route is exempt too: the videos page routes a parent through it,
  // and a captured pin page would redirect to itself forever.
  const isAdmin = user != null && user.role === "admin";
  const onAdminPath = window.location.pathname.startsWith("/admin");
  const onExemptPath =
    onAdminPath || window.location.pathname.startsWith("/pin/");
  const takeover = useTakeover({ pageLoad, onExemptPath });
  usePinSessionPolicy(takeover);
  if (isLoading || (isAuthenticated && pageLoad == null)) {
    return <div className="content-loading"></div>;
  } else if (takeover === "setup") {
    return (
      <Titled title="Set up">
        <SetupView
          token={token}
          apiUrl={apiUrl}
          pageLoad={pageLoad}
          refreshPageLoadData={refreshPageLoadData}
        />
      </Titled>
    );
  } else if (takeover === "videos") {
    return (
      <Titled title="Add videos">
        <VideosRepairView token={token} apiUrl={apiUrl} user={user} />
      </Titled>
    );
  } else {
    // MainView has already short-circuited to content-loading while isLoading,
    // so within this Switch a false isAuthenticated means genuinely logged out.
    return (
      <main>
        <Switch>
          {/*
            "/" and "/privacy" are static pages, not React routes. This entry
            only catches in-app navigations to "/" and hands them back to the
            real document with a full page load.

            Production serves the shell only for the routes enumerated in
            deploy/nginx/mikeymath.conf — a new top-level route added here
            must be added there too, or its deployed URL is a 404.
          */}
          <Route exact path="/">
            <ToLanding />
          </Route>
          <Route exact path="/login">
            <LoginView />
          </Route>
          <Route exact path="/pin/:redirect_pathname">
            <RequireAuth isAuthenticated={isAuthenticated}>
              <Titled title="Enter PIN">
                <PinGateRoute user={user} />
              </Titled>
            </RequireAuth>
          </Route>
          <Route exact path="/play">
            <RequireAuth isAuthenticated={isAuthenticated}>
              <Titled title="Play">
                <PlayView
                  token={token}
                  apiUrl={apiUrl}
                  user={user}
                  postEvent={postEvent}
                  interval={conf.event_reporting_interval}
                  refreshPageLoadData={refreshPageLoadData}
                />
              </Titled>
            </RequireAuth>
          </Route>
          <Route exact path="/settings">
            <RequireAuth isAuthenticated={isAuthenticated}>
              <Titled title="Settings">
                <SettingsView
                  token={token}
                  apiUrl={apiUrl}
                  user={user}
                  settings={settings}
                />
              </Titled>
            </RequireAuth>
          </Route>
          <Route exact path="/progress">
            <RequireAuth isAuthenticated={isAuthenticated}>
              <Titled title="Progress">
                <ProgressView token={token} apiUrl={apiUrl} user={user} />
              </Titled>
            </RequireAuth>
          </Route>
          <Route exact path="/admin">
            {isAdmin ? (
              <Titled title="Admin">
                <AdminHomeView />
              </Titled>
            ) : (
              <NotFound />
            )}
          </Route>
          <Route exact path="/admin/difficulty-calibration">
            {isAdmin ? (
              <Titled title="Difficulty calibration">
                <DifficultyCalibrationView
                  token={token}
                  apiUrl={apiUrl}
                  user={user}
                />
              </Titled>
            ) : (
              <NotFound />
            )}
          </Route>
          <Route exact path="/admin/bitmap-matrix">
            {isAdmin ? (
              <Titled title="Bitmap matrix">
                <BitmapMatrixView token={token} apiUrl={apiUrl} user={user} />
              </Titled>
            ) : (
              <NotFound />
            )}
          </Route>
          <Route exact path="/admin/style-guide">
            {isAdmin ? (
              <Titled title="Style Guide">
                <StyleGuideView />
              </Titled>
            ) : (
              <NotFound />
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
  // One object, so the payload is never half-applied: React 16 does not batch
  // the updates an await lands on, and every consumer of a field here would
  // otherwise have to survive the renders where the rest is still missing.
  const [pageLoad, setPageLoad] = useState(null);
  // Phone-only nav disclosure. Above the breakpoint the nav is always inline
  // and this flag is inert.
  const [navOpen, setNavOpen] = useState(false);

  const genPostEventFcn = useCallback(() => {
    return async function (event_type, value) {
      try {
        console.log("reporting " + event_type + ":" + String(value));
        const req = await apiFetch(ApiUrl, "/events", token, {
          method: "POST",
          body: JSON.stringify({
            event_type: event_type,
            value: String(value),
          }),
        });
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

  // A read that fails leaves the previous payload in place, identity and all,
  // so a caller waiting on fresh data simply never sees it arrive.
  const refreshPageLoadData = useCallback(async () => {
    try {
      if (token == null || user == null) {
        return;
      }
      const pageLoadPath = "/pageload/" + encodeURIComponent(user.sub);
      var req = await apiFetch(ApiUrl, pageLoadPath, token);
      if (req.status === 404) {
        await apiFetch(ApiUrl, "/users", token, {
          method: "POST",
          body: JSON.stringify({
            auth0_id: user.sub,
            email: user.email,
            username: user.name,
          }),
        });
        req = await apiFetch(ApiUrl, pageLoadPath, token);
      }
      // An error body parses as JSON just as happily as a payload does, and
      // its missing fields would land as undefined/NaN, overwriting good state.
      if (!req.ok) {
        return;
      }
      const json = await req.json();
      setPageLoad({
        user: json["user"],
        settings: json["settings"],
        numEnabledVideos: parseInt(json["num_videos_enabled"]),
      });
    } catch (e) {
      console.log(e.message);
    }
  }, [token, user]);

  useEffect(() => {
    refreshPageLoadData();
  }, [refreshPageLoadData]);

  return (
    <div id="react-body">
      <div id="main-menu" className={navOpen ? "nav-open" : ""}>
        <div className="menu-wrap">
          <a href="/">
            <h3>Mikey's Math Game</h3>
          </a>

          {/* Phone-only trigger; CSS hides it above the breakpoint, where the
              nav sits inline. Bare icon, so the name has to come from the
              label. */}
          <button
            className="menu-toggle"
            type="button"
            aria-label="Menu"
            aria-expanded={navOpen}
            aria-controls="main-nav"
            onClick={() => setNavOpen((open) => !open)}
          >
            <span className="menu-bars" aria-hidden="true">
              <span />
              <span />
              <span />
            </span>
          </button>

          <ul className="menu" id="main-nav">
            <li>
              {isAuthenticated ? (
                <button onClick={() => (window.location.pathname = "progress")}>
                  Progress
                </button>
              ) : (
                <></>
              )}
              {isAuthenticated ? (
                <button onClick={() => (window.location.pathname = "settings")}>
                  Adults
                </button>
              ) : (
                <></>
              )}
              {isAuthenticated && pageLoad && pageLoad.user.role === "admin" ? (
                <button onClick={() => (window.location.pathname = "/admin")}>
                  Admin
                </button>
              ) : (
                <></>
              )}
              {isAuthenticated ? <LogoutButton /> : <LoginButton />}
            </li>
          </ul>
        </div>
      </div>

      <div id="content">
        <MainView
          token={token}
          apiUrl={ApiUrl}
          isLoading={isLoading}
          isAuthenticated={isAuthenticated}
          pageLoad={pageLoad}
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
            <a href="/privacy">privacy</a>
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
  document.getElementById("react"),
);
