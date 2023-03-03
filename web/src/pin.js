import React, { useState } from "react";
import { useParams } from 'react-router-dom'
import PinInput from 'react-pin-input';

import './pin.scss'

const pinSessionStorageName = "math-game-pin"

const SetSessionPin = function(pin) {
  sessionStorage.setItem(pinSessionStorageName, pin);
}
const RequirePin = function(correctPin) {
  let sessionPin = sessionStorage.getItem(pinSessionStorageName);
  let valid = false;
  if (sessionPin !== null) {
    valid = sessionPin !== correctPin;
  }
  if (!valid) {
    ClearSessionPin();
    window.location.pathname = "pin/" + encodeURIComponent(window.location.pathname);
  }
  return valid;
}
const ClearSessionPin = function() {
  sessionStorage.removeItem(pinSessionStorageName);
}

const PinView = ({ user, setSessionPin }) => {
  const [error, setError] = useState(user.pin.length < 4);
  const { redirect_pathname } = useParams();

  const handlePinChange = (pin) => {
    let err = pin.length < 4 || pin !== user.pin;
    setError(err);
    if (err) {
      return;
    }
    SetSessionPin(pin);
    window.location.pathname = decodeURIComponent(redirect_pathname);
  };

  return (<>
    <div className="pin-form">
      <h4><span className={error ? "error" : ""}>Enter your four digit PIN code.</span></h4>
      <PinInput 
        autoSelect={true}
        inputMode="number"
        inputStyle={{borderRadius: '0.25em'}}
        length={4} 
        onChange={(value, index) => {handlePinChange(value);}}
        type="numeric"
      />
    </div>
  </>)
}

export {
  SetSessionPin,
  RequirePin,
  ClearSessionPin,
  PinView
}
