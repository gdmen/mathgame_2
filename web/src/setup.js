import React, { useState, useEffect } from "react";

import {
  ProblemTypesSettingsView,
  PlaylistsSettingsView,
  VideosSettingsView,
} from "./settings.js";
import { ProblemTypes } from "./enums.js";
import { GetSessionPin, PinView } from "./pin.js";
import "./settings.scss";
import "./setup.scss";

// Grade-to-default-problem-type mapping (matches curriculum.json on the server)
const GRADE_DEFAULTS = {
  1: ["addition", "subtraction"],
  2: ["addition", "subtraction"],
  3: ["addition", "subtraction", "multiplication", "division"],
  4: ["addition", "subtraction", "multiplication", "division", "fractions"],
  5: ["addition", "subtraction", "multiplication", "division", "fractions"],
  6: ["addition", "subtraction", "multiplication", "division", "fractions", "negatives"],
  7: ["addition", "subtraction", "multiplication", "division", "fractions", "negatives", "word"],
  8: ["addition", "subtraction", "multiplication", "division", "fractions", "negatives", "word"],
};

const GRADE_OPTIONS = [
  { value: 1, label: "1st Grade" },
  { value: 2, label: "2nd Grade" },
  { value: 3, label: "3rd Grade" },
  { value: 4, label: "4th Grade" },
  { value: 5, label: "5th Grade" },
  { value: 6, label: "6th Grade" },
  { value: 7, label: "7th Grade" },
  { value: 8, label: "8th Grade" },
];

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

const GradeLevelTabView = ({ token, apiUrl, user, settings, advanceSetup }) => {
  const [gradeLevel, setGradeLevel] = useState(settings.grade_level || 0);

  const handleGradeSelect = (grade) => {
    setGradeLevel(grade);
    settings.grade_level = grade;
    // Auto-set problem types based on grade
    const defaults = GRADE_DEFAULTS[grade] || ["addition"];
    let bitmap = 0;
    for (const name of defaults) {
      if (ProblemTypes[name]) {
        bitmap += ProblemTypes[name];
      }
    }
    settings.problem_type_bitmap = bitmap;
    postSettings(token, apiUrl, settings);
  };

  const handleSubmitClick = () => {
    if (gradeLevel > 0) {
      advanceSetup();
    }
  };

  return (
    <>
      <h2>Hi there! What grade is your child in?</h2>
      <div className="settings-form">
        <div id="grade-buttons" style={{ display: "flex", flexWrap: "wrap", gap: "8px", justifyContent: "center", margin: "16px 0" }}>
          {GRADE_OPTIONS.map((opt) => (
            <button
              key={opt.value}
              className={gradeLevel === opt.value ? "grade-button active" : "grade-button"}
              onClick={() => handleGradeSelect(opt.value)}
              style={{
                padding: "12px 20px",
                fontSize: "16px",
                border: gradeLevel === opt.value ? "2px solid #4CAF50" : "2px solid #ddd",
                borderRadius: "8px",
                background: gradeLevel === opt.value ? "#E8F5E9" : "#fff",
                cursor: "pointer",
              }}
            >
              {opt.label}
            </button>
          ))}
        </div>
        {gradeLevel > 0 && (
          <p className="settings-hint" style={{ textAlign: "center" }}>
            Problems will be aligned to {GRADE_OPTIONS.find(o => o.value === gradeLevel)?.label} math curriculum.
          </p>
        )}
      </div>
      <button
        className={gradeLevel === 0 ? "submit error" : "submit"}
        onClick={handleSubmitClick}
      >
        continue
      </button>
    </>
  );
};

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
      <h2>Fine-tune which types of problems to show</h2>
      <p className="settings-hint" style={{ textAlign: "center" }}>
        We pre-selected types based on the grade level. Adjust if needed.
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
  const hasEnoughVideos = numEnabledVideos != null && numEnabledVideos >= 1;
  return (
    <>
      <h2>You're all set!</h2>
      <div className="setup-form">
        {!hasEnoughVideos && (
          <p className="settings-hint">
            Add at least 1 YouTube playlist in the Add Videos step to play.
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
    "Grade Level",
    "Problem Types",
    "Add Videos",
    "Set Parent Pin",
    "Start Playing!",
  ];

  if (activeTab == null) {
    setActiveTab("Grade Level");
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
      {activeTab === "Grade Level" && (
        <div className="tab-content">
          <GradeLevelTabView
            token={token}
            apiUrl={apiUrl}
            user={user}
            settings={settings}
            advanceSetup={advanceSetup}
          />
        </div>
      )}
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
          />
        </div>
      )}
    </div>
  );
};

export { SetupView };
