import React, { useEffect, useRef, useState } from "react";
import ReactPlayer from 'react-player'

import './video.scss'

const VideoView = ({ video, postEvent, interval }) => {
  const [playing, setPlaying] = useState(false);
  const [elapsed, setElapsed] = useState(0);

  const elapsedRef = useRef();
  useEffect(() => {
    elapsedRef.current = elapsed;
  }, [elapsed]);

  if (video == null || postEvent == null || interval == null) {
    return <div className="content-loading">loading</div>
  }

  const play = () => {
    setPlaying(true);
  };

  return (
    <div id="video">
      <ReactPlayer
        className='react-player'
        width="100%"
        height="100%"
        url={video.url}
        playing={playing}
        progressInterval={interval}
        onProgress={(e) => {
          var playedMillis = 1000 * e.playedSeconds;
          postEvent("watching_video", playedMillis - elapsedRef.current);
          setElapsed(playedMillis);
        }}
        onEnded={() => {
          postEvent("done_watching_video", video.id);
          window.location.href="play";
        }}
      />
      <div id="click-blocker" onClick={play}></div>
    </div>
  )
}

export {
  VideoView
}
