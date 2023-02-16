import React from "react";
import { SignupButton } from './auth0.js';

import "./home.scss";

const HomeView = ({ isLoading, isAuthenticated, user }) => {
  if (isLoading) {
    return (
      <div className="content-loading">loading</div>
    )
  }
  else if (!isAuthenticated) {
    return (
      <div id="landing-hero">
        <div className="hero-content">
          <div className="hero-copy">
            <h1>Have fun learning math!</h1>
            <p>The Math Game is a simple and easy way for kids to practice math!<br/>Step 1: solve some math problems<br/>Step 2: watch a youtube video as a reward!</p>
            <p>You can set up your child's account in 5 minutes and then watch their progress!</p>
            <div className="button-container">
              <SignupButton />
            </div>
          </div>
          <div className="hero-image">
          </div>
        </div>
      </div>
    )
  }
  else if (!user) {
    return (
      <div className="content-loading">loading</div>
    )
  }

  // User is logged in; redirect to /play
  window.location.href="play";
}

export {
  HomeView
}
