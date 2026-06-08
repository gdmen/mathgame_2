import React, { useCallback, useEffect, useState } from "react";
import { useAuth0 } from "@auth0/auth0-react";
import PinInput from "react-pin-input";

import { ProblemTypes } from "./enums.js";
import { RequirePin, ClearSessionPin } from "./pin.js";
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

// Add public YouTube playlist links to show as "Recommended playlists" (UI only).
const RECOMMENDED_PLAYLISTS = [];

const PlaylistsSettingsView = ({ token, apiUrl, user, onPlaylistsChange }) => {
  const [myPlaylists, setMyPlaylists] = useState([]);
  const [playlistInput, setPlaylistInput] = useState("");
  const [playlistError, setPlaylistError] = useState(null);
  const [addingPlaylist, setAddingPlaylist] = useState(false);

  const authHeaders = () => ({
    Accept: "application/json",
    "Content-Type": "application/json",
    Authorization: "Bearer " + token,
  });

  const fetchMyPlaylists = useCallback(async () => {
    if (token == null || apiUrl == null || user == null) return;
    try {
      const req = await fetch(apiUrl + "/playlists", {
        method: "GET",
        headers: authHeaders(),
      });
      if (req.ok) {
        const json = await req.json();
        setMyPlaylists(Array.isArray(json) ? json : []);
      }
    } catch (e) {
      console.log(e.message);
    }
  }, [token, apiUrl, user]);

  useEffect(() => {
    fetchMyPlaylists();
  }, [fetchMyPlaylists]);

  const handleAddPlaylistByUrl = async (e) => {
    setPlaylistError(null);
    const urlOrId = playlistInput.trim();
    if (!urlOrId) return;
    setAddingPlaylist(true);
    try {
      const body = urlOrId.startsWith("http")
        ? { playlist_url: urlOrId }
        : { youtube_playlist_id: urlOrId };
      const req = await fetch(apiUrl + "/playlists", {
        method: "POST",
        headers: authHeaders(),
        body: JSON.stringify(body),
      });
      const data = req.ok ? await req.json().catch(() => ({})) : null;
      if (req.ok) {
        setPlaylistInput("");
        fetchMyPlaylists();
        if (onPlaylistsChange) onPlaylistsChange();
      } else {
        setPlaylistError(
          (data && (data.message || data.error)) ||
            "Playlist must be public or check the URL."
        );
      }
    } catch (e) {
      setPlaylistError("Could not add playlist. Try again.");
    } finally {
      setAddingPlaylist(false);
    }
  };

  const handleRemovePlaylist = async (playlistId) => {
    try {
      const req = await fetch(apiUrl + "/playlists/" + playlistId, {
        method: "DELETE",
        headers: authHeaders(),
      });
      if (req.ok) {
        fetchMyPlaylists();
        if (onPlaylistsChange) onPlaylistsChange();
      }
    } catch (e) {
      console.log(e.message);
    }
  };

  return (
    <>
      <div className="settings-form" id="playlists-settings">
        <h4>Your playlists</h4>
        <p className="settings-hint">
          Add YouTube playlists; reward videos will be chosen from the union of
          all your playlists.
        </p>
        {playlistError && (
          <p className="error playlist-error">{playlistError}</p>
        )}
        <div id="playlist-inputs">
          <input
            type="text"
            placeholder="YouTube playlist URL or ID (e.g. PLxxx)"
            value={playlistInput}
            onChange={(e) => {
              setPlaylistInput(e.target.value);
              setPlaylistError(null);
            }}
          />
          <button
            type="button"
            onClick={handleAddPlaylistByUrl}
            disabled={addingPlaylist}
            aria-busy={addingPlaylist}
          >
            {addingPlaylist ? "Adding…" : "Add playlist"}
          </button>
        </div>
        <ul id="playlist-list">
          <li className="playlist-list-header">
            <span className="playlist-thumbnail"> </span>
            <span className="playlist-title">PLAYLIST</span>
          </li>
          {myPlaylists.map((p) => (
            <li key={p.id} className="playlist-item">
              <span
                className="playlist-thumbnail"
                style={{
                  backgroundImage: p.thumbnailurl
                    ? `url(${p.thumbnailurl})`
                    : "none",
                }}
              />
              <a
                href={
                  "https://www.youtube.com/playlist?list=" +
                  (p.you_tube_id || "")
                }
                target="_blank"
                rel="noopener noreferrer"
                className="playlist-title"
              >
                {p.title || p.you_tube_id || "Playlist " + p.id}
              </a>
              <span
                className="playlist-remove"
                onClick={() => handleRemovePlaylist(p.id)}
              >
                x
              </span>
            </li>
          ))}
        </ul>
        {RECOMMENDED_PLAYLISTS.length > 0 && (
          <div className="curated-section">
            <h4>Recommended playlists</h4>
            <p className="settings-hint">
              Public YouTube playlists you can add. Paste the URL above and
              click Add playlist, or open the link to view on YouTube.
            </p>
            <ul id="recommended-playlist-list">
              {RECOMMENDED_PLAYLISTS.map((p, i) => (
                <li key={i} className="recommended-playlist-item">
                  <a
                    href={p.url}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="recommended-link"
                  >
                    {p.label}
                  </a>
                  <button
                    type="button"
                    className="add-recommended"
                    onClick={() => setPlaylistInput(p.url)}
                  >
                    Use this URL
                  </button>
                </li>
              ))}
            </ul>
          </div>
        )}
      </div>
    </>
  );
};

const GRADE_OPTIONS = [
  { value: 0, label: "Not set" },
  { value: 1, label: "1st Grade" },
  { value: 2, label: "2nd Grade" },
  { value: 3, label: "3rd Grade" },
  { value: 4, label: "4th Grade" },
  { value: 5, label: "5th Grade" },
  { value: 6, label: "6th Grade" },
  { value: 7, label: "7th Grade" },
  { value: 8, label: "8th Grade" },
];

const GradeLevelSettingsView = ({ token, apiUrl, user, settings }) => {
  const [gradeLevel, setGradeLevel] = useState(settings.grade_level || 0);

  const handleChange = (e) => {
    const val = parseInt(e.target.value);
    setGradeLevel(val);
    settings.grade_level = val;
    postSettings(token, apiUrl, settings);
  };

  return (
    <div id="grade-level-settings" className="settings-form">
      <h4>Grade Level</h4>
      <p className="settings-hint">
        Problems will be aligned to this grade's math curriculum.
      </p>
      <select value={gradeLevel} onChange={handleChange}>
        {GRADE_OPTIONS.map((opt) => (
          <option key={opt.value} value={opt.value}>
            {opt.label}
          </option>
        ))}
      </select>
    </div>
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

function videoPlayUrl(video) {
  if (video.url) return video.url;
  if (video.you_tube_id)
    return "https://www.youtube.com/watch?v=" + video.you_tube_id;
  return "#";
}

const VideosSettingsView = ({
  token,
  apiUrl,
  user,
  errCallback,
  refreshKey,
}) => {
  const [error, setError] = useState(true);
  const [videos, setVideos] = useState([]);

  const getEnabledVideoCount = (list) => list.filter((v) => !v.disabled).length;

  useEffect(() => {
    const getVideos = async () => {
      try {
        if (token == null || apiUrl == null || user == null) return;
        const req = await fetch(apiUrl + "/videos", {
          method: "GET",
          headers: {
            Accept: "application/json",
            "Content-Type": "application/json",
            Authorization: "Bearer " + token,
          },
        });
        if (req.ok) {
          const json = await req.json();
          const list = Array.isArray(json) ? json : [];
          setVideos(list);
          const numEnabled = getEnabledVideoCount(list);
          setError(numEnabled < 3);
          if (errCallback) errCallback(numEnabled < 3);
        }
      } catch (e) {
        console.log(e.message);
      }
    };
    getVideos();
  }, [token, apiUrl, user, errCallback, refreshKey]);

  return (
    <>
      <div className="settings-form">
        <h4>
          Videos from your playlists{" "}
          <span className={error ? "error" : ""}>(at least three)</span>
        </h4>
        <p className="settings-hint">
          These are the reward videos (union of the playlists you added above).
          To add or remove videos, manage the playlists on YouTube or remove a
          playlist above.
        </p>
        <ul id="video-list">
          <li id="video-list-header">
            <span className="video-number">#</span>
            <span className="video-title">TITLE</span>
          </li>
          {videos.map((video, i) => (
            <li key={video.id} className={video.disabled ? "disabled" : ""}>
              <span className="video-number">{i + 1}</span>
              <span
                className="video-thumbnail"
                style={{
                  backgroundImage: video.thumbnailurl
                    ? `url(${video.thumbnailurl})`
                    : "none",
                }}
              >
                <a
                  className="video-play"
                  href={videoPlayUrl(video)}
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
            </li>
          ))}
        </ul>
      </div>
    </>
  );
};

const DeleteAccountView = ({ token, apiUrl, user }) => {
  const { logout } = useAuth0();
  const [showModal, setShowModal] = useState(false);
  const [pin, setPin] = useState("");
  const [error, setError] = useState(null);
  const [submitting, setSubmitting] = useState(false);

  const openModal = () => {
    setPin("");
    setError(null);
    setShowModal(true);
  };

  const closeModal = () => {
    if (submitting) return;
    setShowModal(false);
  };

  const handleDelete = async () => {
    setSubmitting(true);
    setError(null);
    try {
      const req = await fetch(
        apiUrl + "/users/" + encodeURIComponent(user.auth0_id),
        {
          method: "DELETE",
          headers: {
            Accept: "application/json",
            "Content-Type": "application/json",
            Authorization: "Bearer " + token,
          },
          body: JSON.stringify({ pin }),
        }
      );
      if (req.status === 204) {
        // Account is gone; drop the adult PIN session and log out of Auth0.
        ClearSessionPin();
        logout({ returnTo: window.location.origin });
        return;
      }
      if (req.status === 403) {
        setError("Incorrect PIN. Please try again.");
      } else {
        setError("Couldn't delete your account. Please try again.");
      }
    } catch (e) {
      console.log(e.message);
      setError("Couldn't delete your account. Please try again.");
    }
    setSubmitting(false);
  };

  return (
    <div id="delete-account-settings" className="settings-form">
      <h4>Delete account</h4>
      <p className="settings-hint">
        Permanently delete this account and all of its data — settings,
        playlists, videos, and progress. This can&rsquo;t be undone.
      </p>
      <button className="delete-account-button" onClick={openModal}>
        Delete account
      </button>

      {showModal && (
        <div className="delete-account-modal-overlay" onClick={closeModal}>
          <div
            className="delete-account-modal"
            onClick={(e) => e.stopPropagation()}
          >
            <h4>Delete account?</h4>
            <p className="delete-account-modal-copy">
              This permanently deletes the account and all of its data. Enter
              your PIN to confirm.
            </p>
            <div className="delete-account-modal-pin">
              <label htmlFor="delete-account-pin">PIN</label>
              <PinInput
                length={4}
                type="numeric"
                inputMode="number"
                onChange={(value) => setPin(value)}
                inputStyle={{ borderRadius: "0.25em" }}
              />
            </div>
            {error && <p className="delete-account-modal-error">{error}</p>}
            <div className="delete-account-modal-actions">
              <button
                type="button"
                className="delete-account-confirm"
                onClick={handleDelete}
                disabled={submitting || pin.length < 4}
              >
                {submitting ? "Deleting…" : "Delete forever"}
              </button>
              <button type="button" onClick={closeModal} disabled={submitting}>
                Cancel
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
};

const SettingsView = ({ token, apiUrl, user, settings }) => {
  const [videosRefreshKey, setVideosRefreshKey] = useState(0);
  if (!RequirePin(user.id)) {
    return <div className="content-loading"></div>;
  }
  return (
    <div id="settings" className="settings">
      <h2>Settings</h2>
      <div className="tab-content">
        <GradeLevelSettingsView
          token={token}
          apiUrl={apiUrl}
          user={user}
          settings={settings}
        />
      </div>

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
        <PlaylistsSettingsView
          token={token}
          apiUrl={apiUrl}
          user={user}
          onPlaylistsChange={() => setVideosRefreshKey((k) => k + 1)}
        />
      </div>

      <div className="tab-content">
        <VideosSettingsView
          token={token}
          apiUrl={apiUrl}
          user={user}
          errCallback={(e) => null}
          refreshKey={videosRefreshKey}
        />
      </div>

      <div className="tab-content">
        <DeleteAccountView token={token} apiUrl={apiUrl} user={user} />
      </div>
    </div>
  );
};

export {
  ProblemTypesSettingsView,
  PlaylistsSettingsView,
  VideosSettingsView,
  SettingsView,
};
