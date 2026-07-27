import React, {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import ReactPlayer from "react-player";

import "./video.scss";

const VideoView = ({ video, eventReporter, interval }) => {
  const [playing, setPlaying] = useState(false);
  const [elapsed, setElapsed] = useState(0);

  const elapsedRef = useRef();
  useEffect(() => {
    elapsedRef.current = elapsed;
  }, [elapsed]);

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
          url={playUrl}
          playing={playing}
          progressInterval={interval}
          onProgress={(e) => {
            var playedMillis = 1000 * e.playedSeconds;
            eventReporter.postEvent(
              "watching_video",
              playedMillis - elapsedRef.current
            );
            setElapsed(playedMillis);
          }}
          onEnded={() => {
            eventReporter
              .postEvent("done_watching_video", video.id)
              .then(() => {
                window.location.pathname = "play";
              });
          }}
          onError={(e) => {
            eventReporter.postEvent("error_playing_video", e).then(() => {
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
