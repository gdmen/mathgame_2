import React, { useState, useEffect, useRef } from "react";

import { apiFetch } from "./api.js";
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
    const req = await apiFetch(apiUrl, "/settings/" + model.user_id, token, {
      method: "POST",
      body: JSON.stringify(model),
    });
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

// The playlists editor plus an action gated on the video floor. The wizard's
// videos step and the repair page are this one control with different exits.
//
// The playlists view owns the playable-video tally, so the gate reads it from
// there rather than deriving a second one. What it reports is the server's
// de-duplicated total, which is what makes this gate agree with the wizard's
// last step — see docs/accounts.md.
const PlaylistsFloorGate = ({
  token,
  apiUrl,
  user,
  actionLabel,
  onContinue,
}) => {
  const [playableCount, setPlayableCount] = useState(0);
  const error = playableCount < MIN_PLAYABLE_VIDEOS;

  const handleSubmitClick = (e) => {
    // The .error class only stops the pointer, so a keyboard press lands here
    // regardless.
    if (error) return;
    onContinue();
  };

  return (
    <>
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
        {actionLabel}
      </button>
    </>
  );
};

const VideosTabView = ({ token, apiUrl, user, advanceSetup }) => (
  <>
    <h2>Add a YouTube playlist for your child!</h2>
    <PlaylistsFloorGate
      token={token}
      apiUrl={apiUrl}
      user={user}
      actionLabel="continue"
      onContinue={advanceSetup}
    />
  </>
);

const PinTabView = ({ token, apiUrl, user, advanceSetup }) => {
  // Live from the start only when a code already exists — a parent who set
  // one earlier this run and stepped back sees it prefilled (PinView's
  // initialValue) and can continue through; first arrival types all four.
  const [error, setError] = useState(user.pin.length < 4);

  const errCallback = (e) => {
    setError(e);
  };

  const postUser = async function (user) {
    try {
      const req = await apiFetch(
        apiUrl,
        "/users/" + encodeURIComponent(user.auth0_id),
        token,
        { method: "POST", body: JSON.stringify(user) }
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
    // The session PIN is only set when four digits were actually typed here
    // (PinView's handlePinChange). A returning parent passing through with
    // their code prefilled has authored nothing, and writing the session
    // value blind would overwrite their PIN with null.
    const pin = GetSessionPin();
    if (pin != null && pin.length >= 4 && pin !== user.pin) {
      user.pin = pin;
      postUser(user);
    }
    advanceSetup();
  };

  return (
    <>
      <h2>Set a PIN!</h2>
      <p className="settings-hint">
        You'll need it to change these settings later.
      </p>
      <div className="setup-form">
        {/*
          The one caller that authors a code rather than proving one, so it
          opts out of every gate default: nothing to verify against, the
          existing code prefilled for a parent who stepped back to this step,
          and unmasked because they have to read back what they are choosing.
          The step's own heading already names the PIN, so no second prompt.
        */}
        <PinView
          authoring
          prompt={null}
          initialValue={user.pin}
          secret={false}
          errCallback={errCallback}
        />
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
  pageLoad,
  refreshPageLoadData,
  goToVideosStep,
}) => {
  // A parent who reaches this step is finished, so it never argues: the button
  // always goes. What it does instead is check, once, whether the videos are
  // really there, and hand a parent who is short back to the step that fixes it.
  //
  // The payload it mounts with cannot answer that: it is the one the app booted
  // with, and step 2's playlists postdate it. So the answer is whatever payload
  // replaces it — a fresh object every time a read lands, which is also why a
  // read that failed reads as "no answer yet" rather than as the stale count.
  const booted = useRef(pageLoad);
  useEffect(() => {
    refreshPageLoadData();
  }, [refreshPageLoadData]);
  // Fires once. The tab switch unmounts this view, so a second call could not
  // land anyway, but the caller passes a fresh closure on every render and an
  // effect that re-runs on each of them should not be trusted to be harmless.
  const bounced = useRef(false);
  useEffect(() => {
    if (bounced.current || pageLoad === booted.current) return;
    if (pageLoad.numEnabledVideos < MIN_PLAYABLE_VIDEOS && goToVideosStep) {
      bounced.current = true;
      goToVideosStep();
    }
  }, [pageLoad, goToVideosStep]);
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

// Which whole-screen takeover owns this page load, if any: "setup" (the
// first-run wizard — an account with no PIN yet), "videos" (a finished
// account whose reward pool fell below the floor, possibly with no one
// touching anything — YouTube making videos private), or null (the routes).
//
// It latches, because each takeover's own edits are what satisfy the condition
// that raised it: the wizard's steps write the PIN, the repair page's adds
// refill the pool. Re-reading live would tear either off the screen mid-use —
// step 4 refreshes the page-load data, and the last step would vanish before
// the parent could press Start Playing. Every way out of both is a real
// navigation, which starts a fresh latch.
//
// The latch is why the payload arrives as one object: a decision made from a
// half-filled one would stick for the whole page load.
//
// onExemptPath: admin pages (an admin can use them without completing setup)
// and the /pin gate route, which the other PIN-gated pages redirect to — while
// the pool is short, capturing it would render this takeover instead of the
// entry the redirect was for.
const useTakeover = ({ pageLoad, onExemptPath }) => {
  const latched = useRef(null);
  if (latched.current == null && pageLoad != null && !onExemptPath) {
    if (pageLoad.user.pin === "") {
      latched.current = "setup";
    } else if (pageLoad.numEnabledVideos < MIN_PLAYABLE_VIDEOS) {
      latched.current = "videos";
    }
  }
  return latched.current;
};

// The purpose-built repair page for a finished account: just the playlists
// and the way back into the game. It carries the same PIN requirement as
// /settings — it is the same playlist editor /settings keeps behind the PIN,
// and the account holding it finished setup, so the code is already
// provisioned — but the gate renders inline, under this page's own heading,
// rather than bouncing through the /pin route: the person at the screen
// should read why they are suddenly being asked for a code.
const VideosRepairView = ({ token, apiUrl, user }) => {
  // sessionStorage is not reactive; entering the code flips this to re-render
  // past the gate. It always starts locked regardless of any cached session:
  // this page stands in for /play, which is where the device changes hands.
  const [unlocked, setUnlocked] = useState(false);
  return (
    <div id="videos-repair" className="settings">
      <h2>Add videos to keep playing!</h2>
      {unlocked ? (
        <PlaylistsFloorGate
          token={token}
          apiUrl={apiUrl}
          user={user}
          actionLabel="Start Playing!"
          onContinue={() => {
            window.location.href = "/play";
          }}
        />
      ) : (
        <PinView verifyAgainst={user.pin} onValid={() => setUnlocked(true)} />
      )}
    </div>
  );
};

const SETUP_TABS = [
  "Problem Types",
  "Add Videos",
  "Set Parent Pin",
  "Start Playing!",
];

const SetupView = ({ token, apiUrl, pageLoad, refreshPageLoadData }) => {
  const { user, settings } = pageLoad;
  const [activeTab, setActiveTab] = useState(SETUP_TABS[0]);

  // The furthest step reached, which is how far the tab bar lets a click
  // jump. Measured from the furthest step rather than the current one so a
  // parent who stepped back can click forward again to anywhere they have
  // already been — only genuinely unvisited steps stay gated behind their
  // predecessors' continue buttons.
  const maxVisited = useRef(0);

  const advanceSetup = function () {
    const next = SETUP_TABS.indexOf(activeTab) + 1;
    maxVisited.current = Math.max(maxVisited.current, next);
    setActiveTab(SETUP_TABS[next]);
  };

  const handleTabClick = (e) => {
    let clickedId = parseInt(e.target.id.slice(-1));
    if (clickedId > maxVisited.current) {
      return;
    }
    setActiveTab(SETUP_TABS[clickedId]);
  };

  return (
    <div id="setup" className="settings">
      <div id="setup-tabs">
        {SETUP_TABS.map(function (tab, i) {
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
            pageLoad={pageLoad}
            refreshPageLoadData={refreshPageLoadData}
            goToVideosStep={() => setActiveTab("Add Videos")}
          />
        </div>
      )}
    </div>
  );
};

export {
  SetupView,
  VideosRepairView,
  PinTabView,
  StartPlayingTabView,
  useTakeover,
};
