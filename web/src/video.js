import React, { useEffect, useRef, useState } from "react";
import ReactPlayer from 'react-player'

import './video.css'

const VideoView = ({ video, postEvent }) => {
  const [playing, setPlaying] = useState(false);
  const [elapsed, setElapsed] = useState(0);

  const elapsedRef = useRef();
  useEffect(() => {
    elapsedRef.current = elapsed;
  }, [elapsed]);

  if (video == null || postEvent == null) {
    return <div id="loading"></div>
  }

  const play = () => {
    setPlaying(true);
  };

  return (
    <div id="video">
      <ReactPlayer
        url={video.url}
        playing={playing}
        onProgress={(p) => {
          postEvent("watching_video", p.playedSeconds - elapsedRef.current);
          setElapsed(p.playedSeconds);
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
