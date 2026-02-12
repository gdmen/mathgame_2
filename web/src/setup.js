import React, { useState, useEffect } from "react";

import {
  ProblemTypesSettingsView,
  PlaylistsSettingsView,
  VideosSettingsView,
} from "./settings.js";
import { GetSessionPin, PinView } from "./pin.js";
import "./settings.scss";
import "./setup.scss";

const ProblemTypesTabView = ({
  token,
  apiUrl,
  user,
  settings,
  advanceSetup,
}) => {
  const [error, setError] = useState(false);

  const errCallback = (e) => {
    setError(e);
  };

  const handleSubmitClick = (e) => {
    // redirect to next setup step
    advanceSetup();
  };

  return (
    <>
      <h2>Hi there! Let's do a little setup for your child!</h2>
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
  const [error, setError] = useState(true);
  const [videosRefreshKey, setVideosRefreshKey] = useState(0);

  const errCallback = (e) => {
    setError(e);
  };

  const handleSubmitClick = (e) => {
    // redirect to next setup step
    advanceSetup();
  };

  return (
    <>
      <PlaylistsSettingsView
        token={token}
        apiUrl={apiUrl}
        user={user}
        onPlaylistsChange={() => setVideosRefreshKey((k) => k + 1)}
      />
      <VideosSettingsView
        token={token}
        apiUrl={apiUrl}
        user={user}
        errCallback={errCallback}
        refreshKey={videosRefreshKey}
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
    // post updated PIN
    user.pin = GetSessionPin();
    postUser(user);
    // redirect to next setup step
    advanceSetup();
  };

  return (
    <>
      <div className="setup-form">
        <h4>
          Set a PIN! You'll need to remember this to edit these settings later!
        </h4>
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

const StartPlayingTabView = ({ numEnabledVideos, refreshPageLoadData }) => {
  useEffect(() => {
    if (refreshPageLoadData) refreshPageLoadData();
  }, [refreshPageLoadData]);
  const hasEnoughVideos = numEnabledVideos != null && numEnabledVideos >= 3;
  return (
    <>
      <h2>You're all set!</h2>
      <div className="setup-form">
        {!hasEnoughVideos && (
          <p className="settings-hint">
            Add at least 3 videos for this user in the Add Videos step to play.
          </p>
        )}
        <h3>
          Mikey's Math Game will start <strong>easy</strong> and get harder to
          match <strong>your child's</strong> math level!
        </h3>
        <button
          id="start-playing-button"
          disabled={!hasEnoughVideos}
          onClick={function (e) {
            if (hasEnoughVideos) window.location.href = "play";
          }}
        >
          Start Playing!
        </button>
      </div>
    </>
  );
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
    "Choose Problems",
    "Add Videos",
    "Set Parent Pin",
    "Start Playing!",
  ];

  if (activeTab == null) {
    setActiveTab("Choose Problems");
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
      {activeTab === "Choose Problems" && (
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
          />
        </div>
      )}
    </div>
  );
};

export { SetupView };
