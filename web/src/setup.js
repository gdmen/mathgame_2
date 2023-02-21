import React, { useEffect, useState } from "react";
import PinInput from 'react-pin-input';

import './setup.scss'

/*

1. Sign up with Auth0
2. options.pin is "" by default
3. set operations for your kid
3. add at least 3 videos your kid will like - validate youtube videos / make sure they load and play
4. set options.pin to chosen pin (repeat to confirm?)
5. PIN IN MEMORY - if goto setup or parent page, keep pin. If goto any other page, purge pin
6. START PLAYING NOW (goto play) vs CONFIGURE THIS FOR YOUR KID! (goto parent/options page)
*/

const OperationsTabView = ({ token, url, user, options, postOptions, advanceSetup }) => {
  const allOperationsMap = new Map([
    ["Addition", "+"],
    ["Subtraction", "-"],
  ]);
  const revAllOperationsMap = new Map([
    ["+", "Addition"],
    ["-", "Subtraction"],
  ]);
  const [error, setError] = useState(false);
  const [operations, setOperations] = useState(options.operations.split(",").map(function(op, i) { return revAllOperationsMap.get(op); }));

  const handleCheckboxChange = (e) => {
    let id = e.target.id;
    let newOptions = [...operations, id];
    if (operations.includes(id)) {
      newOptions = operations.filter(x => x !== id);
    }
    setOperations(newOptions);
    setError(newOptions.length < 1);
  };

  const handleSubmitClick = (e) => {
    // post updated options
    options.operations = operations.map(function(op, i) {
      return allOperationsMap.get(op);
    }).join(",");
    postOptions(options);
    // redirect to next setup step
    advanceSetup();
  };

  return (<>
    <h2>Hi there! Let's do a little setup for your kid!</h2>
    <div className="setup-form">
      <h4>Which types of problems should we show? <span className={error ? "error" : ""}>Select one or more.</span></h4>
      <ul id="operation-buttons">
        {[...allOperationsMap.keys()].map(function(op, i) {
            var id = op;
            return (<li key={id}>
              <input type="checkbox" id={id} onChange={handleCheckboxChange} checked={"checked" ? operations.includes(id) : ""}/>
              <label htmlFor={id}>
                <div className="operation-button">
                  <span>{op}</span>
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
    // TODO: post updated videos
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
    postVideo({"title": videoTitle, "url": videoUrl, "thumbnailurl": videoThumbnail, "enabled": true});
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

const PinTabView = ({ token, url, user, options, postOptions, advanceSetup }) => {
  const [error, setError] = useState(options.pin.length < 4);
  const [pin, setPin] = useState(options.pin);

  const handlePinChange = (pin) => {
    setError(pin.length < 4);
    if (pin.length === 4) {
      setPin(pin);
    }
  };

  const handleSubmitClick = (e) => {
    // post updated options
    options.pin = pin;
    postOptions(options);
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

const SetupView = ({ token, url, user, options }) => {
  const [activeTab, setActiveTab] = useState(null);

  const allTabs = ["Choose Operations", "Add Videos", "Set Parent Pin", "Start Playing!"];

  if (activeTab == null) {
    setActiveTab("Choose Operations");
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

  const postOptions = async function(options) {
      try {
        const settings = {
            method: 'POST',
            headers: {
              'Accept': 'application/json',
              'Content-Type': 'application/json',
              'Authorization': 'Bearer ' + token,
            },
            body: JSON.stringify(options),
        };
        const req = await fetch(url + "/options/" + options.user_id, settings);
        const json = await req.json();
        return json;
      } catch (e) {
        console.log(e.message);
      }
  };

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
    { (activeTab === "Choose Operations") && <div className="tab-content"><OperationsTabView token={token} url={url} user={user} options={options} postOptions={postOptions} advanceSetup={advanceSetup}/></div> }
    { (activeTab === "Add Videos") && <div className="tab-content"><VideosTabView token={token} url={url} user={user} advanceSetup={advanceSetup}/></div> }
    { (activeTab === "Set Parent Pin") && <div className="tab-content"><PinTabView token={token} url={url} user={user} options={options} postOptions={postOptions} advanceSetup={advanceSetup}/></div> }
    { (activeTab === "Start Playing!") && <div className="tab-content"><StartPlayingTabView /></div> }
  </div>)
}

export {
  SetupView
}
