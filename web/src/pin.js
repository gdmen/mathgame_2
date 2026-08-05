import React, { useEffect, useState } from "react";
import { useRouteMatch } from "react-router-dom";
import PinInput from "react-pin-input";

const pinSessionStorageName = "math-game-pin";

const PIN_LENGTH = 4;

// react-pin-input names each input after the digit it holds unless given an
// ariaLabel, so a masked field reads its own contents out loud and an empty one
// has no name at all. One label for all four boxes is what the widget forwards.
const PIN_DIGIT_LABEL = "PIN digit";

// Route patterns whose screens may keep the adult session alive. These are
// matched by the router, not compared as strings, so they have to be the same
// patterns index.js routes on. Adding a protected screen means adding it here;
// adding any other screen needs nothing.
const PIN_PROTECTED_PATHS = ["/settings"];

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

// The session PIN is honoured only while a PIN-protected surface is actually on
// screen. Stated positively and in one place, so a screen is gated by default:
// the earlier shape had three views each remembering their own ClearSessionPin,
// and a kid-facing screen added tomorrow would inherit whatever session the last
// adult left behind unless its author happened to remember too.
//
// Both halves are load-bearing. The route names the surface, but a takeover (the
// setup wizard, the videos repair page) replaces whatever that route would have
// rendered — and the repair page stands in for /play, which is where the device
// changes hands. Keying on the route alone would honour a session behind a
// takeover that happened to intercept /settings.
//
// The router decides whether we are on a protected route, rather than this
// comparing the URL itself. Those are not the same question: <Route exact
// path="/settings"> also renders the settings page for "/settings/" and
// "/SETTINGS", and a string compare called both of those unprotected — which
// cleared the session on the one page it exists to protect, on every render,
// and turned the gate into a redirect loop. Asking the same matcher the routes
// use is what keeps the two answers from drifting apart again.
const usePinSessionPolicy = (takeover = null) => {
  const onProtectedRoute =
    useRouteMatch({ path: PIN_PROTECTED_PATHS, exact: true }) != null;
  const onProtectedSurface = takeover == null && onProtectedRoute;
  useEffect(() => {
    if (!onProtectedSurface) {
      ClearSessionPin();
    }
  }, [onProtectedSurface]);
};

// Four-digit entry. Every prop defaults to gate behaviour — start empty, mask,
// prompt, and verify — so the wizard step, which is the one caller that authors
// a code rather than proving one, is the only place that has to say so.
//
// `verifyAgainst` has no default on purpose. Skipping verification is something
// a caller has to ask for by passing `authoring`; omitting the code to check
// against leaves `pin !== undefined` always true, so a gate written without it
// rejects every entry rather than accepting every entry. That direction is the
// whole point — the failure mode of forgetting a prop here has to be a gate
// that won't open, not one that always does.
//
// It deliberately knows nothing about routes: `onValid` is the caller's, which
// is what lets the /pin route own its own redirect.
const PinView = ({
  prompt = "Enter your four digit PIN code.",
  initialValue = "",
  verifyAgainst,
  authoring = false,
  secret = true,
  onValid = () => void 0,
  errCallback = () => void 0,
}) => {
  // Errored from the start only when there is no usable code to work with: an
  // account with no PIN at the gate (nothing it could match), or a wizard step
  // with nothing prefilled. Typing is what moves it after that.
  const reference = authoring ? initialValue : verifyAgainst || "";
  const [error, setError] = useState(reference.length < PIN_LENGTH);

  const handlePinChange = (pin) => {
    const newError =
      pin.length < PIN_LENGTH || (!authoring && pin !== verifyAgainst);
    setError(newError);
    errCallback(newError);
    if (newError) {
      return;
    }
    SetSessionPin(pin);
    onValid(pin);
  };

  return (
    <div className="pin-form">
      {/*
        Where this renders alone it is the page's only prompt and its only error
        cue. The wizard's step already names the PIN in its own heading, so
        there it would land as a second title restating the first.
      */}
      {prompt && (
        <h4>
          <span className={error ? "error" : ""}>{prompt}</span>
        </h4>
      )}
      <PinInput
        autoSelect={true}
        focus={true}
        initialValue={initialValue}
        inputMode="numeric"
        inputStyle={{ borderRadius: "0.25em" }}
        length={PIN_LENGTH}
        onChange={(value, index) => {
          handlePinChange(value);
        }}
        ariaLabel={PIN_DIGIT_LABEL}
        secret={secret}
        type="numeric"
      />
    </div>
  );
};

export {
  PIN_DIGIT_LABEL,
  PIN_LENGTH,
  PIN_PROTECTED_PATHS,
  SetSessionPin,
  GetSessionPin,
  RequirePin,
  ClearSessionPin,
  usePinSessionPolicy,
  PinView,
};
