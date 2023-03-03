import React, { useEffect, useState } from "react";

import { ProblemTypes } from './enums.js'
import './settings.scss'

// TODO: clean this file up when I come back to pull views out for the settings page

const ProblemTypesSettingsView = ({ token, url, user, settings }) => {
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
    let newError = newBitmap < 1;
    setError(newError);
    if (!newError) {
      // post updated settings
      settings.problem_type_bitmap = newBitmap;
      postSettings(settings);
    }
  };

  return (<>
    <h2>Settings</h2>
    <div className="settings-form">
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
    </div>
  </>)
}

const VideosSettingsView = ({ token, url, user }) => {
  const [error, setError] = useState(true);
  const [addError, setAddError] = useState(true);
  const [videos, setVideos] = useState(new Map());
  const [videoUrl, setVideoUrl] = useState("");
  const [videoTitle, setVideoTitle] = useState("");
  const [videoThumbnail, setVideoThumbnail] = useState("");

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
          const json = await req.json();
          setVideos(videos => new Map(videos.set(json.url, json)));
          setAddError(false);
          setError(videos.size < 3);
          if (video.url === videoUrl) {
            // If the video we just added is currently in the add video UX
            setVideoUrl("");
            setVideoTitle(null);
            setVideoThumbnail(null);
            setAddError(true);
          }
        }
      } catch (e) {
        console.log(e.message);
      }
  };

  const handleAddVideoClick = (e) => {
    postVideo({"user_id": user.id, "title": videoTitle, "url": videoUrl, "thumbnailurl": videoThumbnail});
  };

  const deleteVideo = async function(video) {
      try {
        const settings = {
            method: 'DELETE',
            headers: {
              'Accept': 'application/json',
              'Content-Type': 'application/json',
              'Authorization': 'Bearer ' + token,
            },
        };
        const req = await fetch(url + "/videos/" + video.id, settings);
        if (req.ok) {
          videos.delete(video.url);
          if (video.url === videoUrl) {
            // If we're deleting the video currently in the add video UX
            setAddError(false);
          }
          setError(videos.size < 3);
          setVideos(new Map(videos));
        }
      } catch (e) {
        console.log(e.message);
      }
  };

  const handleDeleteVideoClick = (e) => {
    deleteVideo(videos.get(e.target.getAttribute("data-video-url")));
  };

  return (<>
    <div className="settings-form">
      <h4>Add <span className={error ? "error" : ""}>at least three</span> <a href="http://www.youtube.com" target="_blank" rel="noopener noreferrer">YouTube</a> videos that your kid will love!</h4>
      <div id="video-inputs">
        <input type="text" placeholder="paste a YouTube link here" className={addError && videoUrl ? "error" : ""} value={videoUrl} onChange={handleAddVideoChange}/>
        <button className={addError ? "error" : ""} onClick={handleAddVideoClick}>add</button>
      </div>
      <ul id="video-list">
        <li id="new-video">
          <span className="video-number"></span>
          <span className="video-thumbnail" style={{
            backgroundImage: `url(${videoThumbnail})` 
          }}></span>
          <span className="video-title">{videoTitle}</span>
        </li>
        <li id="video-list-header">
          <span className="video-number">#</span>
          <span className="video-title">TITLE</span>
        </li>
        {[...videos.keys()].map(function(key, i) {
          var id = i+1;
          var video = videos.get(key);
          return (<li key={id}>
            <span className="video-number">{id}</span>
            <span className="video-thumbnail" style={{
              backgroundImage: `url(${video.thumbnailurl})` 
            }}><a className="video-play" href={video.url} target="_blank" rel="noopener noreferrer" >&#9654;</a></span>
            <span className="video-title">{video.title}</span>
            <span className={videos.size < 3+1 ? "disabled video-delete" : "video-delete"} data-video-url={video.url} onClick={handleDeleteVideoClick}>x</span>
          </li>)
        })}
      </ul>
    </div>
  </>)
}

const SettingsView = ({ token, url, user, settings }) => {
  return (<div id="settings">
    <div className="tab-content"><ProblemTypesSettingsView token={token} url={url} user={user} settings={settings} /></div>
    
    <div className="tab-content"><VideosSettingsView token={token} url={url} user={user} /></div>
  </div>)
}

export {
  ProblemTypesSettingsView,
  VideosSettingsView,
  SettingsView
}