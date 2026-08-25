import React, {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import ReactPlayer from "react-player";

import "./video.scss";

import { EventTypes } from "./enums.generated.js";

// react-player picks the embed domain from the src host, so this is what keeps
// the reward video on the no-cookie domain.
const NOCOOKIE_HOST = "www.youtube-nocookie.com";

const VideoView = ({ video, eventReporter, interval }) => {
  const [playing, setPlaying] = useState(false);
  const [elapsed, setElapsed] = useState(0);

  const elapsedRef = useRef();
  useEffect(() => {
    elapsedRef.current = elapsed;
  }, [elapsed]);

  // timeupdate fires on the player's own polling cadence, which is faster than
  // the reporting interval the caller asked for.
  const lastReportRef = useRef(0);

  const playPause = useCallback(() => {
    setPlaying((wasPlaying) => !wasPlaying);
  }, []);

  // addEventListener with a matching cleanup, rather than assigning
  // document.body.onkeyup: assignment silently replaces whatever else owned the
  // handler and leaves it installed after this view is gone.
  useEffect(() => {
    if (video == null) return;
    const onKeyUp = (e) => {
      if (e.key === " " || e.code === "Space" || e.keyCode === 32) {
        playPause();
      }
    };
    document.addEventListener("keyup", onKeyUp);
    return () => document.removeEventListener("keyup", onKeyUp);
  }, [video, playPause]);

  // Derived, not written back onto the prop: a single video should play, not
  // the playlist it came from.
  const playUrl = useMemo(() => {
    if (video == null) return null;
    const u = new URL(video.url);
    u.searchParams.delete("list");
    u.hostname = NOCOOKIE_HOST;
    return u.toString();
  }, [video]);

  if (video == null || eventReporter == null || interval == null) {
    return <div className="content-loading"></div>;
  }

  return (
    <div id="video-container">
      <div id="video">
        <ReactPlayer
          className="react-player"
          width="100%"
          height="100%"
          src={playUrl}
          playing={playing}
          onTimeUpdate={(e) => {
            const now = Date.now();
            if (now - lastReportRef.current < interval) {
              return;
            }
            lastReportRef.current = now;
            const playedMillis = 1000 * e.currentTarget.currentTime;
            eventReporter.postEvent(
              EventTypes.WATCHING_VIDEO,
              playedMillis - elapsedRef.current,
            );
            setElapsed(playedMillis);
          }}
          onEnded={() => {
            eventReporter
              .postEvent(EventTypes.DONE_WATCHING_VIDEO, video.id)
              .then(() => {
                window.location.pathname = "play";
              });
          }}
          onError={(e) => {
            const err = e.currentTarget && e.currentTarget.error;
            eventReporter
              .postEvent(
                EventTypes.ERROR_PLAYING_VIDEO,
                err ? err.code : e.type,
              )
              .then(() => {
                window.location.pathname = "play";
              });
          }}
        />
        <div id="click-blocker" onClick={playPause}></div>
      </div>
    </div>
  );
};

export { VideoView };
