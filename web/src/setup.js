import React, { useState, useEffect, useRef } from "react";

import {
  MIN_PLAYABLE_VIDEOS,
  ProblemTypesSettingsView,
  PlaylistsSettingsView,
} from "./settings.js";
import { GetSessionPin, PinView } from "./pin.js";
import "./settings.scss";
import "./setup.scss";

const postSettings = async function (token, apiUrl, model) {
  try {
    const reqParams = {
      method: "POST",
      headers: {
        Accept: "application/json",
        "Content-Type": "application/json",
        Authorization: "Bearer " + token,
      },
      body: JSON.stringify(model),
    };
    const req = await fetch(apiUrl + "/settings/" + model.user_id, reqParams);
    const json = await req.json();
    return json;
  } catch (e) {
    console.log(e);
  }
};

const ProblemTypesTabView = ({
  token,
  apiUrl,
  user,
  settings,
  advanceSetup,
}) => {
  const [error, setError] = useState(settings.problem_type_bitmap < 1);

  const errCallback = (e) => {
    setError(e);
  };

  const handleSubmitClick = (e) => {
    // validateBitmap gates continue (errCallback tracks validity)
    if (error) return;
    advanceSetup();
  };

  return (
    <>
      <h2>What kinds of math can your child do?</h2>
      <p className="settings-hint" style={{ textAlign: "center" }}>
        Turn on what your child can do — leave off what they can't yet. You can
        change everything later.
      </p>
      <ProblemTypesSettingsView
        token={token}
        apiUrl={apiUrl}
        user={user}
        settings={settings}
        errCallback={errCallback}
      />
      <button
        className={error ? "submit error" : "submit"}
        onClick={handleSubmitClick}
      >
        continue
      </button>
    </>
  );
};

const VideosTabView = ({ token, apiUrl, user, advanceSetup }) => {
  // The playlists view owns the playable-video tally, so the gate reads it from
  // there rather than deriving a second one. What it reports is the server's
  // de-duplicated total, which is what makes this gate agree with the last
  // step's — see docs/accounts.md.
  const [playableCount, setPlayableCount] = useState(0);
  const error = playableCount < MIN_PLAYABLE_VIDEOS;

  const handleSubmitClick = (e) => {
    // The .error class only stops the pointer, so a keyboard press lands here
    // regardless.
    if (error) return;
    advanceSetup();
  };

  return (
    <>
      <h2>Add a YouTube playlist for your child!</h2>
      <PlaylistsSettingsView
        token={token}
        apiUrl={apiUrl}
        user={user}
        onPlayableCountChange={setPlayableCount}
      />
      <button
        className={error ? "submit error" : "submit"}
        onClick={handleSubmitClick}
      >
        continue
      </button>
    </>
  );
};

const PinTabView = ({ token, apiUrl, user, advanceSetup }) => {
  const [error, setError] = useState(true);

  const errCallback = (e) => {
    setError(e);
  };

  const postUser = async function (user) {
    try {
      const reqParams = {
        method: "POST",
        headers: {
          Accept: "application/json",
          "Content-Type": "application/json",
          Authorization: "Bearer " + token,
        },
        body: JSON.stringify(user),
      };
      const req = await fetch(
        apiUrl + "/users/" + encodeURIComponent(user.auth0_id),
        reqParams
      );
      const json = await req.json();
      return json;
    } catch (e) {
      console.log(e.message);
    }
  };

  const handleSubmitClick = (e) => {
    // Same keyboard hole as the videos step, and here it would write a
    // half-entered PIN to the server before moving on.
    if (error) return;
    user.pin = GetSessionPin();
    postUser(user);
    advanceSetup();
  };

  return (
    <>
      <h2>Set a PIN!</h2>
      <p className="settings-hint">
        You'll need it to change these settings later.
      </p>
      <div className="setup-form">
        <PinView user={user} isSetup={true} errCallback={errCallback} />
        <button
          className={error ? "submit error" : "submit"}
          onClick={handleSubmitClick}
        >
          continue
        </button>
      </div>
    </>
  );
};

const StartPlayingTabView = ({
  numEnabledVideos,
  refreshPageLoadData,
  goToVideosStep,
}) => {
  // A parent who reaches this step is finished, so it never argues: the button
  // always goes. What it does instead is check, once, whether the videos are
  // really there, and hand a parent who is short back to the step that fixes it.
  //
  // The count it is handed cannot answer that on its own. It is the one the app
  // booted with, and nothing between that boot and here refetches it — step 2's
  // playlists postdate it — so acting on it before the refresh lands would send
  // every new account back to step 2 for the length of a request. A refresh that
  // *failed* is not an answer either: it leaves the same stale count behind, and
  // being bounced on the strength of a request that never arrived is worse than
  // being let through. Both cases stay put.
  const [countChecked, setCountChecked] = useState(false);
  useEffect(() => {
    if (!refreshPageLoadData) return;
    let live = true;
    Promise.resolve(refreshPageLoadData()).then(
      (landed) => live && landed !== false && setCountChecked(true),
      () => {}
    );
    return () => {
      live = false;
    };
  }, [refreshPageLoadData]);
  // Fires once. The tab switch unmounts this view, so a second call could not
  // land anyway, but the caller passes a fresh closure on every render and an
  // effect that re-runs on each of them should not be trusted to be harmless.
  const bounced = useRef(false);
  useEffect(() => {
    if (bounced.current || !countChecked || numEnabledVideos == null) return;
    if (numEnabledVideos < MIN_PLAYABLE_VIDEOS && goToVideosStep) {
      bounced.current = true;
      goToVideosStep();
    }
  }, [countChecked, numEnabledVideos, goToVideosStep]);
  return (
    <>
      <h2>You're all set!</h2>
      <div className="setup-form">
        <p className="setup-pitch">
          Mikey's Math Game starts <strong>easy</strong> and adapts to{" "}
          <strong>your child's</strong> math level as they play.
        </p>
        <button
          id="start-playing-button"
          onClick={function (e) {
            window.location.href = "/play";
          }}
        >
          Start Playing!
        </button>
      </div>
    </>
  );
};

// Whether the wizard owns the screen instead of the app's routes.
//
// It latches on, because the wizard's own steps are what satisfy the underlying
// condition (step 2 adds the playlists, step 3 sets the PIN). Re-reading it
// mid-flow would tear the last step off the screen the moment step 4 refreshes
// the page-load data, before the parent has read it or pressed Start Playing.
// Every way out of the wizard is a real navigation, which starts a fresh latch.
//
// The latch is also why an unknown video count has to read as "no answer yet"
// rather than as zero: the caller fills the pageload fields in separate updates,
// so a bare comparison would open the gate on the pass where the count is still
// null and then hold it there for a fully set-up account.
const useSetupGate = ({ user, settings, numEnabledVideos, onAdminPath }) => {
  const latched = useRef(false);
  const needsSetup =
    settings != null &&
    !onAdminPath &&
    (user.pin === "" ||
      (numEnabledVideos != null && numEnabledVideos < MIN_PLAYABLE_VIDEOS));
  if (needsSetup) latched.current = true;
  return latched.current;
};

const SetupView = ({
  token,
  apiUrl,
  user,
  settings,
  numEnabledVideos,
  refreshPageLoadData,
}) => {
  const [activeTab, setActiveTab] = useState(null);

  const allTabs = [
    "Problem Types",
    "Add Videos",
    "Set Parent Pin",
    "Start Playing!",
  ];

  if (activeTab == null) {
    setActiveTab("Problem Types");
  }

  const advanceSetup = function () {
    setActiveTab(allTabs[allTabs.indexOf(activeTab) + 1]);
  };

  const handleTabClick = (e) => {
    let clickedId = parseInt(e.target.id.slice(-1));
    if (clickedId > allTabs.indexOf(activeTab)) {
      return;
    }
    setActiveTab(allTabs[clickedId]);
  };

  return (
    <div id="setup" className="settings">
      <div id="setup-tabs">
        {allTabs.map(function (tab, i) {
          var id = "tab" + i;
          var className = tab === activeTab ? "tab active" : "tab";
          return (
            <div key={id} className={className}>
              <div
                id={id}
                className="tab-click-catcher"
                onClick={handleTabClick}
              ></div>
              <span className="number">{i + 1}</span>
              <span className="label">{tab}</span>
            </div>
          );
        })}
      </div>
      {activeTab === "Problem Types" && (
        <div className="tab-content">
          <ProblemTypesTabView
            token={token}
            apiUrl={apiUrl}
            user={user}
            settings={settings}
            advanceSetup={advanceSetup}
          />
        </div>
      )}
      {activeTab === "Add Videos" && (
        <div className="tab-content">
          <VideosTabView
            token={token}
            apiUrl={apiUrl}
            user={user}
            advanceSetup={advanceSetup}
          />
        </div>
      )}
      {activeTab === "Set Parent Pin" && (
        <div className="tab-content">
          <PinTabView
            token={token}
            apiUrl={apiUrl}
            user={user}
            advanceSetup={advanceSetup}
          />
        </div>
      )}
      {activeTab === "Start Playing!" && (
        <div className="tab-content">
          <StartPlayingTabView
            numEnabledVideos={numEnabledVideos}
            refreshPageLoadData={refreshPageLoadData}
            goToVideosStep={() => setActiveTab("Add Videos")}
          />
        </div>
      )}
    </div>
  );
};

export { SetupView, StartPlayingTabView, useSetupGate };
