import React, { useEffect, useRef, useState } from "react";
import ReactPlayer from "react-player";

import "./video.scss";

const VideoView = ({ video, eventReporter, interval }) => {
  const [playing, setPlaying] = useState(false);
  const [elapsed, setElapsed] = useState(0);

  const elapsedRef = useRef();
  useEffect(() => {
    elapsedRef.current = elapsed;
  }, [elapsed]);

  if (video == null || eventReporter == null || interval == null) {
    return <div className="content-loading"></div>;
  }

  const playPause = () => {
    setPlaying(!playing);
    /* Uncomment to test faster */
    //if (!playing && elapsed > 1000) {
    //  eventReporter.postEvent("done_watching_video", video.id);
    //  window.location.pathname="play";
    //}
  };

  document.body.onkeyup = function (e) {
    if (e.key === " " || e.code === "Space" || e.keyCode === 32) {
      playPause();
    }
  };

  // Remove the playlist parameter from the video url
  var u = new URL(video.url);
  u.searchParams.delete("list");
  video.url = u.toString();

  return (
    <div id="video-container">
      <div id="video">
        <ReactPlayer
          className="react-player"
          width="100%"
          height="100%"
          url={video.url}
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
