import React from "react";
import { SignupButton } from "./auth0.js";

import heroImage from "./img/hero.png";
import { ClearSessionPin } from "./pin.js";

import "./home.scss";

const HomeView = ({ isLoading, isAuthenticated, user, settings }) => {
  ClearSessionPin();

  return (
    <div id="landing-hero">
      <div className="hero-content">
        <div className="hero-copy">
          <h1>Have fun learning math!</h1>
          <p>
            The Math Game is a simple and easy way for kids to practice math!
            <br />
            Step 1: solve some math problems
            <br />
            Step 2: watch a youtube video as a reward!
          </p>
          <p>
            You can set up your child's account in 5 minutes and then watch
            their progress!
          </p>
          <div className="button-container">
            {(isAuthenticated && user && settings && (
              <button
                className="signup"
                onClick={() => (window.location.pathname = "play")}
              >
                <h3>Play Now !</h3>
              </button>
            )) || <SignupButton />}
          </div>
        </div>
        <div
          className="hero-image"
          style={{ backgroundImage: `url(${heroImage})` }}
        ></div>
      </div>
    </div>
  );
};

export { HomeView };
