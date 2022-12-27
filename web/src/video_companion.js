import React, { useState } from "react";
import ReactPlayer from 'react-player'

import './video.css'

const VideoCompanionView = ({ video }) => {
  const [playing, setPlaying] = useState(false);
  
  if (video == null) {
    return <div id="loading">loading</div>
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
      />
      <div id="click-blocker" onClick={play}></div>
    </div>
  )
}

export {
  VideoCompanionView
}
