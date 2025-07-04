import React, { useEffect, useState } from "react";

import { ProblemTypes } from "./enums.js";
import { RequirePin } from "./pin.js";
import "./settings.scss";

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

const ProblemTypesSettingsView = ({
  token,
  apiUrl,
  user,
  settings,
  errCallback,
}) => {
  const [error, setError] = useState(settings.problem_type_bitmap < 1);
  const [problemTypeBitmap, setProblemTypeBitmap] = useState(
    settings.problem_type_bitmap
  );

  useEffect(() => {
    errCallback(error);
  }, [errCallback, error]);

  const handleCheckboxChange = (e) => {
    let newBitmap =
      problemTypeBitmap +
      (2 * e.target.checked - 1) * ProblemTypes[e.target.id];
    setProblemTypeBitmap(newBitmap);
    let newError = newBitmap < 1;
    setError(newError);
    errCallback(newError);
    if (!newError) {
      // post updated settings
      settings.problem_type_bitmap = newBitmap;
      postSettings(token, apiUrl, settings);
    }
  };

  return (
    <>
      <div id="problem-types-settings" className="settings-form">
        <h4>
          Which types of problems should we show?{" "}
          <span className={error ? "error" : ""}>Select one or more.</span>
        </h4>
        <ul id="problem-type-buttons">
          {Object.keys(ProblemTypes).map(function (problemType, i) {
            return (
              <li key={problemType}>
                <input
                  type="checkbox"
                  id={problemType}
                  onChange={handleCheckboxChange}
                  checked={
                    "checked"
                      ? (ProblemTypes[problemType] & problemTypeBitmap) > 0
                      : ""
                  }
                />
                <label htmlFor={problemType}>
                  <div className="problem-type-button">
                    <span>{problemType}</span>
                  </div>
                </label>
              </li>
            );
          })}
        </ul>
      </div>
    </>
  );
};

const TargetWorkPercentageSettingsView = ({
  token,
  apiUrl,
  user,
  settings,
}) => {
  const [targetWorkPercentage, setTargetWorkPercentage] = useState(
    settings.target_work_percentage
  );

  const handleChange = (e) => {
    let val = e.target.value;
    setTargetWorkPercentage(val);
    settings.target_work_percentage = parseInt(val);
  };

  const handleSubmit = (e) => {
    // post updated settings
    postSettings(token, apiUrl, settings);
  };

  return (
    <>
      <div id="target-work-percentage-settings" className="settings-form">
        <h4>Percentage of time doing math:</h4>
        <div>{targetWorkPercentage} %</div>
        <input
          type="range"
          value={targetWorkPercentage}
          onChange={handleChange}
          onMouseUp={handleSubmit}
          onBlur={handleSubmit}
        />
      </div>
    </>
  );
};

const VideosSettingsView = ({ token, apiUrl, user, errCallback }) => {
  const [error, setError] = useState(true);
  const [addError, setAddError] = useState(true);
  const [videos, setVideos] = useState(new Map());
  const [videoUrl, setVideoUrl] = useState("");
  const [videoTitle, setVideoTitle] = useState("");
  const [videoThumbnail, setVideoThumbnail] = useState("");

  const getEnabledVideoCount = (videos) => {
    return new Map([...videos].filter(([k, v]) => !v.disabled)).size;
  };

  useEffect(() => {
    const getVideos = async () => {
      try {
        if (token == null || apiUrl == null || user == null) {
          return;
        }
        const reqParams = {
          method: "GET",
          headers: {
            Accept: "application/json",
            "Content-Type": "application/json",
            Authorization: "Bearer " + token,
          },
        };
        const req = await fetch(apiUrl + "/videos", reqParams);
        const json = await req.json();

        let newVideos = new Map();
        for (var i in json) {
          let url = json[i].url;
          newVideos.set(url, json[i]);
        }
        setVideos(newVideos);
        var numEnabled = getEnabledVideoCount(newVideos);
        setError(numEnabled < 3);
        errCallback(numEnabled < 3);
      } catch (e) {
        console.log(e.message);
      }
    };

    getVideos();
  }, [token, apiUrl, user, errCallback]);

  const fetchYouTubeMetadata = async function (url, okFcn, errFcn) {
    try {
      const reqParams = {
        method: "GET",
        headers: {
          Accept: "application/json",
          "Content-Type": "application/json",
        },
      };
      const req = await fetch(
        "https://www.youtube.com/oembed?format=json&url=" +
          encodeURIComponent(url),
        reqParams
      );
      const json = await req.json();
      okFcn(json);
    } catch (e) {
      console.log(e.message);
      errFcn(e);
    }
  };

  const handleAddVideoChange = (e) => {
    let url = e.target.value;
    // Remove the playlist parameter from the video url
    var u = new URL(url);
    u.searchParams.delete("list");
    url = u.toString();

    setVideoUrl(url);
    setVideoTitle(null);
    setVideoThumbnail(null);
    let okFcn = function (json) {
      setVideoTitle(json.title);
      setVideoThumbnail(json.thumbnail_url);
      setAddError(videos.has(url));
    };
    let errFcn = function (e) {
      setAddError(true);
    };
    fetchYouTubeMetadata(url, okFcn, errFcn);
  };

  const postVideo = async function (video) {
    try {
      const reqParams = {
        method: "POST",
        headers: {
          Accept: "application/json",
          "Content-Type": "application/json",
          Authorization: "Bearer " + token,
        },
        body: JSON.stringify(video),
      };
      const req = await fetch(apiUrl + "/videos/", reqParams);
      if (req.ok) {
        const json = await req.json();
        var newVideos = new Map(videos.set(json.url, json));
        setVideos(newVideos);
        var numEnabled = getEnabledVideoCount(newVideos);
        setError(numEnabled < 3);
        errCallback(numEnabled < 3);
        setAddError(false);
        if (video.url === videoUrl) {
          // If the video we just added is currently in the add video UI, clear out that UI
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
    postVideo({
      user_id: user.id,
      title: videoTitle,
      url: videoUrl,
      thumbnailurl: videoThumbnail,
    });
  };

  const deleteVideo = async function (video) {
    try {
      const reqParams = {
        method: "DELETE",
        headers: {
          Accept: "application/json",
          "Content-Type": "application/json",
          Authorization: "Bearer " + token,
        },
      };
      const req = await fetch(apiUrl + "/videos/" + video.id, reqParams);
      if (req.ok) {
        videos.delete(video.url);
        if (video.url === videoUrl) {
          // If we're deleting the video currently in the add video UI
          setAddError(false);
        }
        setVideos(new Map(videos));
        var numEnabled = getEnabledVideoCount(videos);
        setError(numEnabled < 3);
        errCallback(numEnabled < 3);
      }
    } catch (e) {
      console.log(e.message);
    }
  };

  const handleDeleteVideoClick = (e) => {
    deleteVideo(videos.get(e.target.getAttribute("data-video-url")));
  };

  return (
    <>
      <div className="settings-form">
        <h4>
          Add <span className={error ? "error" : ""}>at least three</span>{" "}
          <a
            href="http://www.youtube.com"
            target="_blank"
            rel="noopener noreferrer"
          >
            YouTube
          </a>{" "}
          videos that your child will love!
        </h4>
        <div id="video-inputs">
          <input
            type="text"
            placeholder="paste a YouTube link here"
            className={addError && videoUrl ? "error" : ""}
            value={videoUrl}
            onChange={handleAddVideoChange}
          />
          <button
            className={addError ? "error" : ""}
            onClick={handleAddVideoClick}
          >
            add
          </button>
        </div>
        <ul id="video-list">
          <li id="new-video">
            <span className="video-number"></span>
            <span
              className="video-thumbnail"
              style={{
                backgroundImage: `url(${videoThumbnail})`,
              }}
            ></span>
            <span className="video-title">{videoTitle}</span>
          </li>
          <li id="video-list-header">
            <span className="video-number">#</span>
            <span className="video-title">TITLE</span>
          </li>
          {[...videos.keys()].map(function (key, i) {
            var id = i + 1;
            var video = videos.get(key);
            return (
              <li key={id} className={`${video.disabled ? "disabled" : ""}`}>
                <span className="video-number">{id}</span>
                <span
                  className="video-thumbnail"
                  style={{
                    backgroundImage: `url(${video.thumbnailurl})`,
                  }}
                >
                  <a
                    className="video-play"
                    href={video.url}
                    target="_blank"
                    rel="noopener noreferrer"
                  >
                    {video.disabled ? (
                      <span>unavailable</span>
                    ) : (
                      <span>&#9654;</span>
                    )}
                  </a>
                </span>
                <span className="video-title">{video.title}</span>
                <span
                  className={`video-delete ${
                    !video.disabled && videos.size <= 3 ? "disabled" : ""
                  }`}
                  data-video-url={video.url}
                  onClick={handleDeleteVideoClick}
                >
                  x
                </span>
              </li>
            );
          })}
        </ul>
      </div>
    </>
  );
};

const SettingsView = ({ token, apiUrl, user, settings }) => {
  if (!RequirePin(user.id)) {
    return <div className="content-loading"></div>;
  }
  return (
    <div id="settings" className="settings">
      <h2>Settings</h2>
      <div className="tab-content">
        <ProblemTypesSettingsView
          token={token}
          apiUrl={apiUrl}
          user={user}
          settings={settings}
          errCallback={(e) => null}
        />
      </div>

      <div className="tab-content">
        <TargetWorkPercentageSettingsView
          token={token}
          apiUrl={apiUrl}
          user={user}
          settings={settings}
        />
      </div>

      <div className="tab-content">
        <VideosSettingsView
          token={token}
          apiUrl={apiUrl}
          user={user}
          errCallback={(e) => null}
        />
      </div>
    </div>
  );
};

export { ProblemTypesSettingsView, VideosSettingsView, SettingsView };
