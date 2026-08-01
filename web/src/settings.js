import React, { useCallback, useEffect, useRef, useState } from "react";
import { useAuth0 } from "@auth0/auth0-react";
import PinInput from "react-pin-input";

import { ProblemTypes } from "./enums.js";
import { validateBitmap, targetDifficultyRange } from "./bitmap_validation.js";
import { RequirePin, ClearSessionPin } from "./pin.js";
import "./settings.scss";

// Throws on any non-2xx or network failure so callers can surface it. A save
// that fails silently is indistinguishable from one that worked, which is how
// a parent loses settings without knowing.
const postSettings = async function (token, apiUrl, model) {
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
  if (!req.ok) {
    throw new Error("save failed with status " + req.status);
  }
  return req.json();
};

// Drives the per-card save indicator: idle -> saving -> saved (auto-clearing)
// or error, with a retry that replays the last attempt.
const useSaveState = () => {
  const [status, setStatus] = useState("idle");
  const lastAttempt = useRef(null);
  const timerRef = useRef(null);

  useEffect(() => {
    return () => clearTimeout(timerRef.current);
  }, []);

  const run = useCallback(async (fn) => {
    lastAttempt.current = fn;
    clearTimeout(timerRef.current);
    setStatus("saving");
    try {
      await fn();
      setStatus("saved");
      timerRef.current = setTimeout(() => setStatus("idle"), 2000);
    } catch (e) {
      setStatus("error");
    }
  }, []);

  const retry = useCallback(() => {
    if (lastAttempt.current) run(lastAttempt.current);
  }, [run]);

  return { status, run, retry };
};

const SaveState = ({ state }) => {
  if (state.status === "idle") {
    return null;
  }
  if (state.status === "error") {
    return (
      <span className="save-state save-error">
        Not saved
        <button type="button" className="save-retry" onClick={state.retry}>
          Retry
        </button>
      </span>
    );
  }
  return (
    <span className="save-state">
      {state.status === "saving" ? "Saving…" : "Saved ✓"}
    </span>
  );
};

// How far a range input's track is filled. Only the position lives here; the
// colours stay in settings.scss with the rest of the tokens.
const fillTo = (percent) => ({ "--fill-pct": percent + "%" });

// A settings card: the question-titled surface the problem-type groups already
// use, extended to every control on the page.
const SettingsCard = ({ title, question, wide, saveState, children }) => (
  <section className={"settings-card" + (wide ? " settings-card-wide" : "")}>
    <div className="settings-card-head">
      <h4>{title}</h4>
      {saveState && <SaveState state={saveState} />}
    </div>
    {question && <p className="settings-hint">{question}</p>}
    {children}
  </section>
);

// Problem-type taxonomy: each bit is placed by one question -
// verb / noun-kind / noun-size / framing. Labels are parent vocabulary;
// internal constants are feature-named (see server/api/enums.go).
// Dependent entries (dependsOn) always render on their own row directly
// below their parent and are disabled until the parent is on; parents with
// dependents (hasDependent) sit at the bottom of their card. Toggle chips
// never change size on any state change (selection is color-only).
const PROBLEM_TYPE_GROUPS = [
  {
    title: "Operations",
    question: "What can your child do?",
    entries: [
      { bit: ProblemTypes.ADDITION, label: "Addition" },
      { bit: ProblemTypes.SUBTRACTION, label: "Subtraction" },
      { bit: ProblemTypes.DIVISION, label: "Division" },
      {
        bit: ProblemTypes.MULTIPLICATION,
        label: "Multiplication",
        hasDependent: true,
      },
      {
        bit: ProblemTypes.PERCENTAGES,
        label: "Percentages",
        dependsOn: ProblemTypes.MULTIPLICATION,
      },
    ],
  },
  {
    title: "Number types",
    question: "What kinds of numbers?",
    entries: [
      { bit: ProblemTypes.DECIMALS, label: "Decimals" },
      { bit: ProblemTypes.NEGATIVES, label: "Negative numbers" },
      // Parents with dependents sit at the bottom of the card; the
      // dependent renders on its own row directly below.
      { bit: ProblemTypes.FRACTIONS, label: "Fractions", hasDependent: true },
      {
        bit: ProblemTypes.MISMATCHED_DENOMINATORS,
        label: "Different denominators",
        dependsOn: ProblemTypes.FRACTIONS,
      },
    ],
  },
  {
    title: "Number size",
    question: "How big can the numbers be?",
    hint: "Without these, numbers stay 1–12.",
    entries: [
      { bit: ProblemTypes.MEDIUM_NUMBERS, label: "Numbers up to 99" },
      { bit: ProblemTypes.LARGE_NUMBERS, label: "Numbers 100 and up" },
    ],
  },
  {
    title: "Problem format",
    question: "How can problems be posed?",
    entries: [
      { bit: ProblemTypes.WORD, label: "Word problems" },
      {
        bit: ProblemTypes.MISSING_NUMBER,
        label: "Fill in the blank (? + 5 = 12)",
      },
      { bit: ProblemTypes.SINGLE_VARIABLE, label: "Algebra (3x + 7 = 22)" },
      {
        bit: ProblemTypes.CHAINED_OPERATIONS,
        label: "Multi-step (2+ operations)",
        hasDependent: true,
      },
      {
        bit: ProblemTypes.PEMDAS,
        label: "Order of operations",
        dependsOn: ProblemTypes.CHAINED_OPERATIONS,
      },
    ],
  },
];

// applyToggleRules keeps dependent bits coherent when one toggles:
// enabling LARGE auto-enables MEDIUM (no size gap), disabling a parent
// auto-clears its dependents.
const applyToggleRules = (bitmap, bit, enabled) => {
  let b = enabled ? bitmap | bit : bitmap & ~bit;
  if (enabled && bit === ProblemTypes.LARGE_NUMBERS) {
    b |= ProblemTypes.MEDIUM_NUMBERS;
  }
  if (enabled && bit === ProblemTypes.PERCENTAGES) {
    b |= ProblemTypes.MEDIUM_NUMBERS;
  }
  if (!enabled && bit === ProblemTypes.MEDIUM_NUMBERS) {
    b &= ~ProblemTypes.LARGE_NUMBERS;
    b &= ~ProblemTypes.PERCENTAGES;
  }
  if (!enabled && bit === ProblemTypes.FRACTIONS) {
    b &= ~ProblemTypes.MISMATCHED_DENOMINATORS;
  }
  if (!enabled && bit === ProblemTypes.CHAINED_OPERATIONS) {
    b &= ~ProblemTypes.PEMDAS;
  }
  if (!enabled && bit === ProblemTypes.MULTIPLICATION) {
    b &= ~ProblemTypes.PERCENTAGES;
  }
  return b;
};

const ProblemTypesSettingsView = ({
  token,
  apiUrl,
  user,
  settings,
  errCallback,
  onBitmapChange,
}) => {
  const [problemTypeBitmap, setProblemTypeBitmap] = useState(
    settings.problem_type_bitmap
  );
  const saveState = useSaveState();
  const validation = validateBitmap(problemTypeBitmap);

  useEffect(() => {
    errCallback(!validation.valid);
  }, [errCallback, validation.valid]);

  const commit = (newBitmap) => {
    setProblemTypeBitmap(newBitmap);
    const v = validateBitmap(newBitmap);
    errCallback(!v.valid);
    if (onBitmapChange) onBitmapChange(newBitmap);
    if (v.valid) {
      settings.problem_type_bitmap = newBitmap;
      saveState.run(() => postSettings(token, apiUrl, settings));
    }
  };

  const handleToggle = (entry, checked) => {
    commit(applyToggleRules(problemTypeBitmap, entry.bit, checked));
  };

  // Anchor each validation error to the card it concerns.
  const ERROR_GROUPS = {
    NO_CORE_OP: "Operations",
    LARGE_REQUIRES_MEDIUM: "Number size",
    MISMATCHED_REQUIRES_FRACTIONS: "Number types",
    PEMDAS_REQUIRES_CHAINED: "Problem format",
    PERCENTAGES_REQUIRE_MULTIPLICATION: "Operations",
    PERCENTAGES_REQUIRE_MEDIUM: "Operations",
  };
  const errorsFor = (groupTitle) =>
    validation.valid
      ? []
      : validation.errors.filter(
          (err) => ERROR_GROUPS[err.code] === groupTitle
        );

  return (
    <SettingsCard
      title="Skills"
      question="What can your child do, and how can problems be posed?"
      wide
      saveState={saveState}
    >
      <div id="problem-types-settings">
        <div className="problem-type-grid">
          {PROBLEM_TYPE_GROUPS.map((group) => (
            <div key={group.title} className="problem-type-card">
              <h5>{group.title}</h5>
              <p className="settings-hint group-question">{group.question}</p>
              <ul id="problem-type-buttons">
                {group.entries.map((entry) => {
                  const id = "pt-" + entry.bit;
                  const parentOff =
                    entry.dependsOn != null &&
                    (problemTypeBitmap & entry.dependsOn) === 0;
                  const cls =
                    entry.dependsOn != null
                      ? "dep" + (parentOff ? " parent-off" : "")
                      : entry.hasDependent
                      ? "has-dep"
                      : "";
                  return (
                    <li key={entry.bit} className={cls}>
                      <input
                        type="checkbox"
                        id={id}
                        disabled={parentOff}
                        onChange={(e) => handleToggle(entry, e.target.checked)}
                        checked={(entry.bit & problemTypeBitmap) > 0}
                      />
                      <label htmlFor={id}>
                        <div className="problem-type-button">
                          {entry.dependsOn != null && (
                            <span className="dep-arrow">↳</span>
                          )}
                          <span>{entry.label}</span>
                        </div>
                      </label>
                    </li>
                  );
                })}
              </ul>
              {errorsFor(group.title).map((err) => (
                <p key={err.code} className="error">
                  {err.message}
                </p>
              ))}
              {group.hint && (
                <p className="settings-hint group-hint">{group.hint}</p>
              )}
            </div>
          ))}
        </div>
      </div>
    </SettingsCard>
  );
};

// Add public YouTube playlist links to show as "Recommended playlists" (UI only).
const RECOMMENDED_PLAYLISTS = [];

// The reward loop needs at least this many playable videos to draw from.
const MIN_PLAYABLE_VIDEOS = 3;

const playlistName = (p) => p.title || p.you_tube_id || "Playlist " + p.id;

// One playlist row, expandable in place to the videos it contributes. Videos
// load on first open, so a parent with many playlists pays for only what they
// look at.
const PlaylistRow = ({ playlist, apiUrl, authHeaders, onRemove }) => {
  const [videos, setVideos] = useState(null);
  const [error, setError] = useState(null);

  // <details> owns the open/closed state, so the whole row is one hit target
  // and the keyboard behaviour is the browser's. onToggle only mirrors it and
  // triggers the first load.
  const handleToggle = async (e) => {
    if (!e.target.open || videos != null) return;
    try {
      const req = await fetch(
        apiUrl + "/playlists/" + playlist.id + "/videos",
        { method: "GET", headers: authHeaders() }
      );
      if (!req.ok) throw new Error("status " + req.status);
      const json = await req.json();
      setVideos(Array.isArray(json) ? json : []);
    } catch (e) {
      setError("Could not load this playlist's videos.");
    }
  };

  // Remove sits inside the summary for layout, so it has to opt out of the
  // summary's default toggle.
  const handleRemoveClick = (e) => {
    e.preventDefault();
    e.stopPropagation();
    onRemove(playlist);
  };

  const total = playlist.video_count || 0;
  const playable = playlist.playable_count || 0;
  const countLabel =
    playable === total
      ? total + (total === 1 ? " video" : " videos")
      : total + " videos · " + playable + " playable";

  return (
    <li className="playlist-item">
      <details onToggle={handleToggle}>
        <summary className="playlist-row">
          <span className="playlist-caret">▶</span>
          <span
            className="playlist-thumbnail"
            style={{
              backgroundImage: playlist.thumbnailurl
                ? `url(${playlist.thumbnailurl})`
                : "none",
            }}
          />
          <span className="playlist-title">{playlistName(playlist)}</span>
          <span className="playlist-count">{countLabel}</span>
          <button
            type="button"
            className="playlist-remove"
            onClick={handleRemoveClick}
          >
            Remove…
          </button>
        </summary>
        <div className="playlist-videos">
          {error && <p className="error">{error}</p>}
          {!error && videos == null && (
            <p className="settings-hint">Loading…</p>
          )}
          {!error && videos != null && videos.length === 0 && (
            <p className="settings-hint">
              No videos yet. YouTube playlists sync shortly after they're added.
            </p>
          )}
          {!error &&
            videos != null &&
            videos.map((v) => (
              <div
                key={v.id}
                className={"playlist-video" + (v.disabled ? " disabled" : "")}
              >
                <span
                  className="playlist-video-thumbnail"
                  style={{
                    backgroundImage: v.thumbnailurl
                      ? `url(${v.thumbnailurl})`
                      : "none",
                  }}
                />
                <a
                  className="playlist-video-title"
                  href={videoPlayUrl(v)}
                  target="_blank"
                  rel="noopener noreferrer"
                >
                  {v.title}
                </a>
                {v.disabled && (
                  <span className="playlist-video-state">unavailable</span>
                )}
              </div>
            ))}
        </div>
      </details>
    </li>
  );
};

const PlaylistsSettingsView = ({
  token,
  apiUrl,
  user,
  onPlaylistsChange,
  onPlayableCountChange,
}) => {
  const [myPlaylists, setMyPlaylists] = useState([]);
  // Server-side and de-duplicated: a video in two of these playlists is one
  // reward, so the rows' own counts must not be summed into this.
  const [totalPlayable, setTotalPlayable] = useState(0);
  const [playlistInput, setPlaylistInput] = useState("");
  const [playlistError, setPlaylistError] = useState(null);
  const [addingPlaylist, setAddingPlaylist] = useState(false);

  // Stable per token: the playlist rows take it as a prop, and fetchMyPlaylists
  // declares it as a dependency.
  const authHeaders = useCallback(
    () => ({
      Accept: "application/json",
      "Content-Type": "application/json",
      Authorization: "Bearer " + token,
    }),
    [token]
  );

  const fetchMyPlaylists = useCallback(async () => {
    if (token == null || apiUrl == null || user == null) return;
    try {
      const req = await fetch(apiUrl + "/playlists", {
        method: "GET",
        headers: authHeaders(),
      });
      if (req.ok) {
        const json = await req.json();
        setMyPlaylists(Array.isArray(json.playlists) ? json.playlists : []);
        setTotalPlayable(
          Number.isFinite(json.playable_total) ? json.playable_total : 0
        );
      }
    } catch (e) {
      console.log(e.message);
    }
  }, [token, apiUrl, user, authHeaders]);

  useEffect(() => {
    fetchMyPlaylists();
  }, [fetchMyPlaylists]);

  const belowMinimum = totalPlayable < MIN_PLAYABLE_VIDEOS;

  // The setup wizard gates its "continue" on this count.
  useEffect(() => {
    if (onPlayableCountChange) onPlayableCountChange(totalPlayable);
  }, [onPlayableCountChange, totalPlayable]);

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

  // MIN_PLAYABLE_VIDEOS mirrors the floor the reward loop needs; removing a
  // playlist that would breach it warns before it happens rather than leaving
  // the parent to discover it from the video count.
  //
  // `remaining` is a floor, not the answer: videos this playlist shares with
  // another survive its removal but are subtracted here anyway, so the warning
  // can fire on a removal that in fact stays above the line. Erring toward the
  // warning is the safe direction, and the server recount lands right after.
  const handleRemovePlaylist = async (playlist) => {
    const remaining = totalPlayable - (playlist.playable_count || 0);
    const warning =
      remaining < MIN_PLAYABLE_VIDEOS
        ? "Removing “" +
          playlistName(playlist) +
          "” leaves " +
          remaining +
          " playable video" +
          (remaining === 1 ? "" : "s") +
          ", below the " +
          MIN_PLAYABLE_VIDEOS +
          " the game needs to hand out rewards. Remove it anyway?"
        : "Remove “" + playlistName(playlist) + "” from your rewards?";
    if (!window.confirm(warning)) return;
    setPlaylistError(null);
    try {
      const req = await fetch(apiUrl + "/playlists/" + playlist.id, {
        method: "DELETE",
        headers: authHeaders(),
      });
      if (!req.ok) {
        setPlaylistError("Could not remove that playlist. Try again.");
        return;
      }
      fetchMyPlaylists();
      if (onPlaylistsChange) onPlaylistsChange();
    } catch (e) {
      setPlaylistError("Could not remove that playlist. Try again.");
    }
  };

  return (
    <SettingsCard
      title={
        <>
          Playlists{" "}
          <span
            className={
              "playlist-total" + (belowMinimum ? " playlist-total-low" : "")
            }
          >
            {totalPlayable} playable video{totalPlayable === 1 ? "" : "s"}
          </span>
        </>
      }
      question="Rewards are drawn only from these."
      wide
    >
      <div id="playlists-settings">
        {belowMinimum && (
          <p className="error">
            The game needs at least {MIN_PLAYABLE_VIDEOS} playable videos to
            hand out a reward. Add a playlist below.
          </p>
        )}
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
            {addingPlaylist ? "Adding…" : "Add"}
          </button>
        </div>
        <ul id="playlist-list">
          {myPlaylists.length === 0 && (
            <li className="settings-hint">
              No playlists yet. Add one above to give your child something to
              earn.
            </li>
          )}
          {myPlaylists.map((p) => (
            <PlaylistRow
              key={p.id}
              playlist={p}
              apiUrl={apiUrl}
              authHeaders={authHeaders}
              onRemove={handleRemovePlaylist}
            />
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
    </SettingsCard>
  );
};

// TargetDifficultySettingsView: the slider's range IS the envelope - the
// band from the easiest to the hardest problem the current bitmap can
// express (mirrors the server's TargetDifficultyRange; the server clamps
// authoritatively on save). A high-weight envelope (e.g. division-only)
// floors above the global minimum: nothing easier is constructible.
const TargetDifficultySettingsView = ({
  token,
  apiUrl,
  user,
  settings,
  bitmap,
}) => {
  const { lo: floor, hi: ceiling } = targetDifficultyRange(bitmap);
  const [targetDifficulty, setTargetDifficulty] = useState(
    settings.target_difficulty
  );
  const shown = Math.min(Math.max(targetDifficulty, floor), ceiling);
  const saveState = useSaveState();

  // The raw difficulty numbers are formula internals - parents only need
  // the relative position within what the enabled problem types allow,
  // shown as an integer percent (1-100) like the work-percentage slider.
  // Guard the degenerate collapsed band (floor === ceiling): show full.
  const span = ceiling - floor;
  const percent =
    span > 0 ? Math.max(1, Math.round(((shown - floor) / span) * 100)) : 100;

  // This value is the adaptive system's, not the parent's: the readout is a
  // meter and the parent's control is a nudge, so the copy and the affordance
  // finally agree. A nudge steps a tenth of the envelope and clamps to it,
  // exactly as dragging the old slider to that point did.
  const nudge = (direction) => {
    if (span <= 0) return;
    const stepped = shown + direction * span * 0.1;
    const clamped = Math.min(Math.max(stepped, floor), ceiling);
    setTargetDifficulty(clamped);
    settings.target_difficulty = clamped;
    saveState.run(() => postSettings(token, apiUrl, settings));
  };

  return (
    <SettingsCard
      title="Current difficulty"
      question="Adjusts automatically as your child plays; nudge it if it feels off."
      saveState={saveState}
    >
      <div id="target-difficulty-settings">
        <p className="settings-value">{percent}%</p>
        <div
          className="settings-meter"
          role="meter"
          aria-valuenow={percent}
          aria-valuemin={0}
          aria-valuemax={100}
          aria-label="Current difficulty"
        >
          <div
            className="settings-meter-fill"
            style={{ width: percent + "%" }}
          />
        </div>
        <div className="scale-labels">
          <span>easiest</span>
          <span>hardest these settings allow</span>
        </div>
        <div className="settings-nudges">
          <button
            type="button"
            onClick={() => nudge(-1)}
            disabled={span <= 0 || shown <= floor}
          >
            ← Easier
          </button>
          <button
            type="button"
            onClick={() => nudge(1)}
            disabled={span <= 0 || shown >= ceiling}
          >
            Harder →
          </button>
        </div>
      </div>
    </SettingsCard>
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
  const saveState = useSaveState();

  const handleChange = (e) => {
    let val = e.target.value;
    setTargetWorkPercentage(val);
    settings.target_work_percentage = parseInt(val);
  };

  const handleSubmit = () => {
    saveState.run(() => postSettings(token, apiUrl, settings));
  };

  return (
    <SettingsCard
      title="Math / video balance"
      question="How much of a session is math?"
      saveState={saveState}
    >
      <div id="target-work-percentage-settings">
        <p className="settings-value">{targetWorkPercentage}% math</p>
        {/* A native range paints only a track and thumb, so the filled portion
            is drawn as a background gradient — without it this control reads as
            a different component from the difficulty meter beside it. */}
        <input
          className="settings-slider"
          type="range"
          value={targetWorkPercentage}
          aria-label="Percentage of time doing math"
          style={fillTo(targetWorkPercentage)}
          onChange={handleChange}
          onMouseUp={handleSubmit}
          onTouchEnd={handleSubmit}
          onKeyUp={handleSubmit}
          onBlur={handleSubmit}
        />
        <div className="scale-labels">
          <span>more video time</span>
          <span>more math time</span>
        </div>
      </div>
    </SettingsCard>
  );
};

function videoPlayUrl(video) {
  if (video.url) return video.url;
  if (video.you_tube_id)
    return "https://www.youtube.com/watch?v=" + video.you_tube_id;
  return "#";
}

// Deleting is destructive and irreversible, so it is gated twice: the Adults
// PIN already guards this page, and the modal re-asks for it. The server
// verifies the PIN itself, so a stolen token alone can't delete an account.
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
        // The account is gone; drop the adult PIN session and log out of Auth0.
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
    <SettingsCard
      title="Delete account"
      question="Want to remove this account and everything in it?"
      wide
    >
      <div className="delete-account">
        <p className="settings-hint">
          This deletes your account, settings, playlists and saved progress,
          then signs you out. Anonymous gameplay data is kept, with nothing left
          in it that identifies you. Account deletion can&rsquo;t be undone.
        </p>
        <button
          type="button"
          className="delete-account-open"
          onClick={openModal}
        >
          Delete account
        </button>
      </div>

      {showModal && (
        <>
          <div className="report-modal-overlay" onClick={closeModal} />
          <div className="report-modal" onClick={(e) => e.stopPropagation()}>
            <h4>Delete account?</h4>
            <p className="report-modal-copy">
              This permanently deletes the account. Enter your PIN to confirm.
            </p>
            <div className="report-modal-pin">
              <label>Enter PIN to confirm</label>
              <PinInput
                length={4}
                type="numeric"
                inputMode="numeric"
                inputStyle={{ borderRadius: "0.25em" }}
                onChange={(value) => setPin(value)}
                onComplete={() => {}}
              />
            </div>
            {error && <p className="report-modal-error">{error}</p>}
            <div className="report-modal-actions">
              <button type="button" onClick={closeModal} disabled={submitting}>
                Cancel
              </button>
              <button
                type="button"
                className="delete-account-confirm"
                onClick={handleDelete}
                disabled={submitting || pin.length < 4}
                aria-busy={submitting}
              >
                {submitting ? "Deleting…" : "Delete forever"}
              </button>
            </div>
          </div>
        </>
      )}
    </SettingsCard>
  );
};

const SettingsView = ({ token, apiUrl, user, settings }) => {
  const [bitmap, setBitmap] = useState(settings.problem_type_bitmap);
  if (!RequirePin(user.pin)) {
    return <div className="content-loading"></div>;
  }
  return (
    <div id="settings" className="settings">
      <h1 className="settings-header">Settings</h1>
      {/* Every control is a card in one grid, so a parent adjusting one dial
          can see the state of the others without scrolling. Skills leads
          because it defines the envelope the difficulty meter beneath it is
          measured against. */}
      <div className="settings-grid">
        <ProblemTypesSettingsView
          token={token}
          apiUrl={apiUrl}
          user={user}
          settings={settings}
          errCallback={(e) => null}
          onBitmapChange={setBitmap}
        />
        <TargetWorkPercentageSettingsView
          token={token}
          apiUrl={apiUrl}
          user={user}
          settings={settings}
        />
        <TargetDifficultySettingsView
          token={token}
          apiUrl={apiUrl}
          user={user}
          settings={settings}
          bitmap={bitmap}
        />
        <PlaylistsSettingsView token={token} apiUrl={apiUrl} user={user} />
        <DeleteAccountView token={token} apiUrl={apiUrl} user={user} />
      </div>
    </div>
  );
};

export {
  MIN_PLAYABLE_VIDEOS,
  ProblemTypesSettingsView,
  PlaylistsSettingsView,
  DeleteAccountView,
  SettingsView,
};
