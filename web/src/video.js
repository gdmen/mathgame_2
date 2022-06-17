import React, {  useState } from "react";
import ReactPlayer from 'react-player'

import './video.css'

const VideoView = ({ video, postEvent }) => {
  const [playing, setPlaying] = useState(null);

  if (video == null || postEvent == null) {
    return <div></div>
  }

  const play = async () => {
    setPlaying(true);
  };

  return (
    <div id="video">
      <ReactPlayer
        url={video.url}
        playing={playing}
        onEnded={() => { postEvent("done_watching_video", video.id); }}
      />
      <div id="click-blocker" onClick={play}></div>
    </div>
  )
}

export {
  VideoView
}
