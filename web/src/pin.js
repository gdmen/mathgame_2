import React, { useState } from "react";
import { useParams } from "react-router-dom";
import PinInput from "react-pin-input";

import "./pin.scss";

const pinSessionStorageName = "math-game-pin";

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
    valid = sessionPin !== correctPin;
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

const PinView = ({ user, isSetup = false, errCallback = () => void 0 }) => {
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
      window.location.pathname = decodeURIComponent(redirect_pathname);
    }
  };

  return (
    <>
      <div className="pin-form">
        <h4>
          <span className={error ? "error" : ""}>
            Enter your four digit PIN code.
          </span>
        </h4>
        <PinInput
          autoSelect={true}
          focus={true}
          inputMode="number"
          inputStyle={{ borderRadius: "0.25em" }}
          length={4}
          onChange={(value, index) => {
            handlePinChange(value);
          }}
          type="numeric"
        />
      </div>
    </>
  );
};

export { SetSessionPin, GetSessionPin, RequirePin, ClearSessionPin, PinView };
