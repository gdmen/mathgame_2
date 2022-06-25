import React from "react";
import { LoginButton } from './auth0.js'

import "./home.css"

const HomeView = ({ isLoading, isAuthenticated, user }) => {
  if (isLoading) {
    return (
      <div id="home">loading</div>
    )
  }
  else if (!isAuthenticated) {
    return (
      <div id="home">
        <p>Welcome to The Math Game!</p>
        <LoginButton />
      </div>
    )
  }
  else if (!user) {
    return (
      <div id="home">loading</div>
    )
  }

  // User is logged in; redirect to /play
  window.location.href="play";
}

export {
  HomeView
}
