import React, { useState } from "react";

import { ProblemTypesSettingsView, VideosSettingsView } from './settings.js'
import { GetSessionPin, PinView } from './pin.js'
import './settings.scss'
import './setup.scss'

// TODO: clean this file up when I come back to pull views out for the settings page

const ProblemTypesTabView = ({ token, url, user, settings, advanceSetup }) => {
  const [error, setError] = useState(false);

  const errCallback = (e) => {
    setError(e);
  };

  const handleSubmitClick = (e) => {
    // redirect to next setup step
    advanceSetup();
  };

  return (<>
    <h2>Hi there! Let's do a little setup for your kid!</h2>
    <ProblemTypesSettingsView token={token} url={url} user={user} settings={settings} errCallback={errCallback} />
    <button className={error ? "submit error" : "submit"} onClick={handleSubmitClick}>continue</button>
  </>)
}

const VideosTabView = ({ token, url, user, advanceSetup }) => {
  const [error, setError] = useState(true);

  const errCallback = (e) => {
    setError(e);
  };

  const handleSubmitClick = (e) => {
    // redirect to next setup step
    advanceSetup();
  };

  return (<>
    <VideosSettingsView token={token} url={url} user={user} errCallback={errCallback} />
    <button className={error ? "submit error" : "submit"} onClick={handleSubmitClick}>continue</button>
  </>)
}

const PinTabView = ({ token, url, user, advanceSetup }) => {
  const [error, setError] = useState(true);

  const errCallback = (e) => {
    setError(e);
  };

  const postUser = async function(user) {
      try {
        const settings = {
            method: 'POST',
            headers: {
              'Accept': 'application/json',
              'Content-Type': 'application/json',
              'Authorization': 'Bearer ' + token,
            },
            body: JSON.stringify(user),
        };
        const req = await fetch(url + "/users/" + encodeURIComponent(user.auth0_id), settings);
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

  return (<>
    <div className="setup-form">
      <h4>Set a PIN! You'll need to remember this to edit these settings later!</h4>
      <PinView user={user} isSetup={true} errCallback={errCallback} />
      <button className={error ? "submit error" : "submit"} onClick={handleSubmitClick}>continue</button>
    </div>
  </>)
}

const StartPlayingTabView = () => {
  return (<>
    <h2>We're all set!</h2>
    <div className="setup-form">
      <h3>The Math Game will start easy and get harder to match your kid's math level!</h3>
      <button id="start-playing-button" onClick={function(e){window.location.href="play"}}>Start Playing!</button>
    </div>
  </>)
}

const SetupView = ({ token, url, user, settings }) => {
  const [activeTab, setActiveTab] = useState(null);

  const allTabs = ["Choose Problems", "Add Videos", "Set Parent Pin", "Start Playing!"];

  if (activeTab == null) {
    setActiveTab("Choose Problems");
  }

  const advanceSetup = function() {
    setActiveTab(allTabs[allTabs.indexOf(activeTab)+1]);
  }

  const handleTabClick = (e) => {
    let clickedId = parseInt(e.target.id.slice(-1));
    if (clickedId > allTabs.indexOf(activeTab)) {
      return;
    }
    setActiveTab(allTabs[clickedId]);
  }

  return (<div id="setup">
    <div id="setup-tabs">
      {allTabs.map(function(tab, i){
        var id = "tab" + i;
        var className = tab === activeTab ? "tab active" : "tab";
        return (
          <div key={id} className={className}>
            <div id={id} className="tab-click-catcher" onClick={handleTabClick}></div>
            <span className="number">{i+1}</span>
            <span className="label">{tab}</span>
          </div>
        )
      })}
    </div>
    { (activeTab === "Choose Problems") && <div className="tab-content"><ProblemTypesTabView token={token} url={url} user={user} settings={settings} advanceSetup={advanceSetup}/></div> }
    { (activeTab === "Add Videos") && <div className="tab-content"><VideosTabView token={token} url={url} user={user} advanceSetup={advanceSetup}/></div> }
    { (activeTab === "Set Parent Pin") && <div className="tab-content"><PinTabView token={token} url={url} user={user} advanceSetup={advanceSetup} /></div> }
    { (activeTab === "Start Playing!") && <div className="tab-content"><StartPlayingTabView /></div> }
  </div>)
}

export {
  SetupView
}
