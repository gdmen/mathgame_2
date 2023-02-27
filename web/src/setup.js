import React, { useEffect, useState } from "react";
import PinInput from 'react-pin-input';

import { ProblemTypes } from './enums.js'
import './setup.scss'

// TODO: clean this file up when I come back to pull views out for the settings page

const ProblemTypesTabView = ({ token, url, user, settings, advanceSetup }) => {
  const [error, setError] = useState(false);
  const [problemTypeBitmap, setProblemTypeBitmap] = useState(settings.problem_type_bitmap);

  const postSettings = async function(model) {
      try {
        const settings = {
            method: 'POST',
            headers: {
              'Accept': 'application/json',
              'Content-Type': 'application/json',
              'Authorization': 'Bearer ' + token,
            },
            body: JSON.stringify(model),
        };
        const req = await fetch(url + "/settings/" + model.user_id, settings);
        const json = await req.json();
        return json;
      } catch (e) {
        console.log(e.message);
      }
  };

  const handleCheckboxChange = (e) => {
    let newBitmap = problemTypeBitmap + ((2 * e.target.checked -1) * ProblemTypes[e.target.id]);
    setProblemTypeBitmap(newBitmap);
    setError(newBitmap < 1);
  };

  const handleSubmitClick = (e) => {
    // post updated settings
    settings.problem_type_bitmap = problemTypeBitmap;
    postSettings(settings);
    // redirect to next setup step
    advanceSetup();
  };

  return (<>
    <h2>Hi there! Let's do a little setup for your kid!</h2>
    <div className="setup-form">
      <h4>Which types of problems should we show? <span className={error ? "error" : ""}>Select one or more.</span></h4>
      <ul id="problem-type-buttons">
        {Object.keys(ProblemTypes).map(function(problemType, i) {
            return (<li key={problemType}>
              <input type="checkbox" id={problemType} onChange={handleCheckboxChange} checked={"checked" ? ((ProblemTypes[problemType] & problemTypeBitmap) > 0) : ""}/>
              <label htmlFor={problemType}>
                <div className="problem-type-button">
                  <span>{problemType}</span>
                </div>
              </label>
            </li>)
        })}
      </ul>
      <button className={error ? "submit error" : "submit"} onClick={handleSubmitClick}>continue</button>
    </div>
  </>)
}

const VideosTabView = ({ token, url, user, advanceSetup }) => {
  const [error, setError] = useState(true);
  const [addError, setAddError] = useState(true);
  const [videos, setVideos] = useState(new Map());
  const [videoUrl, setVideoUrl] = useState(null);
  const [videoTitle, setVideoTitle] = useState(null);
  const [videoThumbnail, setVideoThumbnail] = useState(null);

  useEffect(() => {
    const getVideos = async () => {
      try {
        if (token == null || url == null || user == null) {
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
        const req = await fetch(url+ "/videos", settings);
        const json = await req.json();
        let newVideos = new Map();
        for (var i in json) {
          let url = json[i].url;
          newVideos.set(url, json[i]);
        }
        setVideos(newVideos);
        setError(newVideos.size < 3);
      } catch (e) {
        console.log(e.message);
      }
    };

    getVideos();
  }, [token, url, user]);

  const handleSubmitClick = (e) => {
    // redirect to next setup step
    advanceSetup();
  };

  const fetchYouTubeMetadata = async function(url, okFcn, errFcn) {
      try {
        const settings = {
            method: 'GET',
            headers: {
              'Accept': 'application/json',
              'Content-Type': 'application/json',
            },
        };
        const req = await fetch("https://www.youtube.com/oembed?format=json&url=" + encodeURIComponent(url), settings);
        const json = await req.json();
        okFcn(json);
      } catch (e) {
        console.log(e.message);
        errFcn(e);
      }
  };

  const handleAddVideoChange = (e) => {
    let url = e.target.value;
    setVideoUrl(url);
    setVideoTitle(null);
    setVideoThumbnail(null);
    let okFcn = function (json) {
      setVideoTitle(json.title);
      setVideoThumbnail(json.thumbnail_url);
      setAddError(videos.has(url));
    }
    let errFcn = function (e) {
      setAddError(true);
    }
    fetchYouTubeMetadata(url, okFcn, errFcn);
  }

  const postVideo = async function(video) {
      try {
        const settings = {
            method: 'POST',
            headers: {
              'Accept': 'application/json',
              'Content-Type': 'application/json',
              'Authorization': 'Bearer ' + token,
            },
            body: JSON.stringify(video),
        };
        const req = await fetch(url + "/videos/", settings);
        if (req.ok) {
          setVideos(videos => new Map(videos.set(video.url, video)));
          setAddError(true);
          setError(videos.size < 3);
        }
      } catch (e) {
        console.log(e.message);
      }
  };

  const handleAddVideoClick = (e) => {
    postVideo({"user_id": user.id, "title": videoTitle, "url": videoUrl, "thumbnailurl": videoThumbnail});
  };

  return (<>
    <div className="setup-form">
      <h4>Add <span className={error ? "error" : ""}>at least three</span> <a href="http://www.youtube.com" target="_blank" rel="noopener noreferrer">YouTube</a> videos that your kid will love!</h4>
      <ul id="video-list">
        {[...videos.keys()].map(function(key, i) {
            var id = i;
            var video = videos.get(key);
            return (<li key={id} style={{
              backgroundImage: `url(${video.thumbnailurl})` 
            }}>
              <span className="video-title">{video.title}</span>
            </li>)
        })}
      </ul>
      <div id="add-video-interface" style={{
          backgroundImage: `url(${videoThumbnail})` 
      }}>
        <div className={addError ? "video-title error": "video-title"}><h3>{videoTitle}</h3></div>
        <div id="video-inputs">
          <input type="text" placeholder="paste a YouTube link here" className={addError && videoUrl ? "error" : ""} onChange={handleAddVideoChange}/>
          <button className={addError ? "error" : ""} onClick={handleAddVideoClick}>add</button>
        </div>
      </div>
      <button className={error ? "submit error" : "submit"} onClick={handleSubmitClick}>continue</button>
    </div>
  </>)
}

const PinTabView = ({ token, url, user, advanceSetup }) => {
  const [error, setError] = useState(user.pin.length < 4);
  const [pin, setPin] = useState(user.pin);

  const handlePinChange = (pin) => {
    setError(pin.length < 4);
    if (pin.length === 4) {
      setPin(pin);
    }
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
    // post updated settings
    user.pin = pin;
    postUser(user);
    // redirect to next setup step
    advanceSetup();
  };

  return (<>
    <div className="setup-form">
      <h4>Set a <span className={error ? "error" : ""}>four digit</span> PIN code! You'll need this PIN to edit these settings later!</h4>
      <PinInput 
        autoSelect={true}
        initialValue={pin}
        inputMode="number"
        inputStyle={{borderRadius: '0.25em'}}
        length={4} 
        onChange={(value, index) => {handlePinChange(value);}}
        type="numeric"
      />
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
    { (activeTab === "Set Parent Pin") && <div className="tab-content"><PinTabView token={token} url={url} user={user} advanceSetup={advanceSetup}/></div> }
    { (activeTab === "Start Playing!") && <div className="tab-content"><StartPlayingTabView /></div> }
  </div>)
}

export {
  SetupView
}
