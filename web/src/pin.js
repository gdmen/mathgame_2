import React, { useState } from "react";
import { useParams } from "react-router-dom";
import PinInput from "react-pin-input";

import "./pin.scss";

const pinSessionStorageName = "math-game-pin";

// react-pin-input names each input after the digit it holds unless given an
// ariaLabel, so a masked field reads its own contents out loud and an empty one
// has no name at all. One label for all four boxes is what the widget forwards.
const PIN_DIGIT_LABEL = "PIN digit";

const SetSessionPin = function (pin) {
  sessionStorage.setItem(pinSessionStorageName, pin);
};
const GetSessionPin = function () {
  return sessionStorage.getItem(pinSessionStorageName);
};
const RequirePin = function (correctPin) {
  let sessionPin = GetSessionPin();
  let valid = false;
  if (sessionPin !== null) {
    valid = sessionPin === correctPin;
  }
  if (!valid) {
    ClearSessionPin();
    window.location.pathname =
      "pin/" + encodeURIComponent(window.location.pathname);
  }
  return valid;
};
const ClearSessionPin = function () {
  sessionStorage.removeItem(pinSessionStorageName);
};

const PinView = ({
  user,
  isSetup = false,
  errCallback = () => void 0,
  onSuccess = null,
}) => {
  const [error, setError] = useState(user.pin.length < 4);
  const { redirect_pathname } = useParams();

  const handlePinChange = (pin) => {
    let newError = pin.length < 4;
    if (!isSetup) {
      newError |= pin !== user.pin;
    }
    setError(newError);
    errCallback(newError);
    if (newError) {
      return;
    }
    SetSessionPin(pin);
    if (!isSetup) {
      // Gate mode has two callers: the /pin route, where success navigates
      // back to the page that redirected here, and an inline gate (the videos
      // repair page), which stays put and just needs to know.
      if (onSuccess) {
        onSuccess();
      } else {
        window.location.pathname = decodeURIComponent(redirect_pathname);
      }
    }
  };

  return (
    <>
      <div className="pin-form">
        {/*
          At the gate route this is the page's only prompt and its only error
          cue. The wizard's step already names the PIN in its own heading, so
          there it would land as a second title restating the first.
        */}
        {!isSetup && (
          <h4>
            <span className={error ? "error" : ""}>
              Enter your four digit PIN code.
            </span>
          </h4>
        )}
        <PinInput
          autoSelect={true}
          focus={true}
          // Prefilled only while authoring: a parent who set a code earlier
          // in this run and stepped back to this step sees it instead of
          // being asked to invent another. The gate route never prefills —
          // typing the code is the entire check.
          initialValue={isSetup ? user.pin : ""}
          inputMode="numeric"
          inputStyle={{ borderRadius: "0.25em" }}
          length={4}
          onChange={(value, index) => {
            handlePinChange(value);
          }}
          ariaLabel={PIN_DIGIT_LABEL}
          // Masked at the gate, plain during setup: the gate is typed with a
          // kid watching the same screen, while setup is the one moment the
          // adult has to be able to read back the code they are choosing.
          secret={!isSetup}
          type="numeric"
        />
      </div>
    </>
  );
};

export {
  PIN_DIGIT_LABEL,
  SetSessionPin,
  GetSessionPin,
  RequirePin,
  ClearSessionPin,
  PinView,
};
