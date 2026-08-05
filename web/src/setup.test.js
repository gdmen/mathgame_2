import React from "react";
import ReactDOM from "react-dom";
import { act } from "react-dom/test-utils";
import { MemoryRouter } from "react-router-dom";

import {
  VideosRepairView,
  PinTabView,
  StartPlayingTabView,
  useTakeover,
} from "./setup.js";
import { SetSessionPin, GetSessionPin, ClearSessionPin } from "./pin.js";

// react-pin-input drives its per-digit focus through real timers, which jsdom
// can't satisfy. Stand in a single input reporting the same thing the widget
// reports (the concatenated value), as delete_account.test.js does.
jest.mock("react-pin-input", () => {
  const mockReact = require("react");
  return {
    __esModule: true,
    default: ({ onChange }) =>
      mockReact.createElement("input", {
        className: "mock-pin",
        onChange: (e) => onChange(e.target.value),
      }),
  };
});

// The takeover decides who owns the screen: the first-run wizard (no PIN
// yet), the PIN-gated videos repair page (finished account, pool below the
// floor), or the routes. It has to hold once it decides — the wizard's own
// steps and the repair page's own edits are what satisfy the underlying
// condition, so re-reading it live would tear either off the screen
// mid-use. Pinned here: which takeover each account state gets, that a
// decision sticks for the page load, and that no decision is made from a
// half-arrived payload.
const Probe = (props) => <i>{useTakeover(props) || "app"}</i>;

const NEW_USER = { pin: "" };
const SET_UP_USER = { pin: "1234" };

const render = (container, props) =>
  act(() => {
    ReactDOM.render(<Probe {...props} />, container);
  });

const shown = (container) => container.querySelector("i").textContent;

let container;
beforeEach(() => {
  container = document.createElement("div");
  document.body.appendChild(container);
});
afterEach(() => {
  ReactDOM.unmountComponentAtNode(container);
  container.remove();
});

test("opens for an account that has not finished setup", () => {
  render(container, {
    user: NEW_USER,
    settings: {},
    numEnabledVideos: 0,
    onExemptPath: false,
  });
  expect(shown(container)).toBe("setup");
});

test("stays open once the wizard's own steps satisfy the gate", () => {
  render(container, {
    user: NEW_USER,
    settings: {},
    numEnabledVideos: 0,
    onExemptPath: false,
  });
  render(container, {
    user: SET_UP_USER,
    settings: {},
    numEnabledVideos: 12,
    onExemptPath: false,
  });
  expect(shown(container)).toBe("setup");
});

test("never opens for an account that is already set up", () => {
  render(container, {
    user: SET_UP_USER,
    settings: {},
    numEnabledVideos: 12,
    onExemptPath: false,
  });
  expect(shown(container)).toBe("app");
});

test("a finished account short on videos gets the repair page, not the wizard", () => {
  render(container, {
    user: SET_UP_USER,
    settings: {},
    numEnabledVideos: 2,
    onExemptPath: false,
  });
  expect(shown(container)).toBe("videos");
});

test("the repair takeover holds while its own edits fix the pool", () => {
  render(container, {
    user: SET_UP_USER,
    settings: {},
    numEnabledVideos: 2,
    onExemptPath: false,
  });
  render(container, {
    user: SET_UP_USER,
    settings: {},
    numEnabledVideos: 12,
    onExemptPath: false,
  });
  expect(shown(container)).toBe("videos");
});

test("stays shut on an exempt path (admin, /pin) with setup unfinished", () => {
  render(container, {
    user: NEW_USER,
    settings: {},
    numEnabledVideos: 0,
    onExemptPath: true,
  });
  expect(shown(container)).toBe("app");
});

test("stays shut before the page-load data arrives", () => {
  render(container, {
    user: null,
    settings: null,
    numEnabledVideos: null,
    onExemptPath: false,
  });
  expect(shown(container)).toBe("app");
});

// `refreshPageLoadData` sets the user, the settings and the video count as three
// separate updates after an await, and this React version does not batch those,
// so the gate is asked to decide on each half-filled pass. The latch makes any
// wrong answer permanent for the page load.
test("stays shut through the renders where the page-load data is half in", () => {
  const SET_UP = { user: SET_UP_USER, settings: {}, numEnabledVideos: 12 };
  render(container, { user: null, settings: null, numEnabledVideos: null });
  render(container, {
    user: SET_UP.user,
    settings: null,
    numEnabledVideos: null,
  });
  render(container, {
    user: SET_UP.user,
    settings: {},
    numEnabledVideos: null,
  });
  render(container, SET_UP);
  expect(shown(container)).toBe("app");
});

// The last step never argues: a parent who reaches it gets through. What it
// does do is check the videos once and, if they are genuinely short, hand them
// back to the step that fixes it — but only on an answer it actually got. The
// count it starts with is the app's boot value, taken before step 2 ran.
const renderLastStep = (container, props) =>
  act(() => {
    ReactDOM.render(<StartPlayingTabView {...props} />, container);
  });

const startButton = (container) =>
  container.querySelector("#start-playing-button");

test("last step lets the parent through", () => {
  renderLastStep(container, { numEnabledVideos: 12 });
  expect(startButton(container).disabled).toBe(false);
  expect(container.querySelector("h2").textContent).toBe("You're all set!");
});

test("the button is live even before the count is known", () => {
  renderLastStep(container, {
    numEnabledVideos: 0,
    refreshPageLoadData: () => new Promise(() => {}),
  });
  expect(startButton(container).disabled).toBe(false);
});

test("a checked count below the floor sends the parent back to the videos step", async () => {
  const goToVideosStep = jest.fn();
  await act(async () => {
    ReactDOM.render(
      <StartPlayingTabView
        numEnabledVideos={2}
        refreshPageLoadData={() => Promise.resolve(true)}
        goToVideosStep={goToVideosStep}
      />,
      container
    );
  });
  expect(goToVideosStep).toHaveBeenCalled();
});

test("a checked count at the floor keeps the parent here", async () => {
  const goToVideosStep = jest.fn();
  await act(async () => {
    ReactDOM.render(
      <StartPlayingTabView
        numEnabledVideos={3}
        refreshPageLoadData={() => Promise.resolve(true)}
        goToVideosStep={goToVideosStep}
      />,
      container
    );
  });
  expect(goToVideosStep).not.toHaveBeenCalled();
});

// The boot count is 0 for a new account, so acting on it before the refresh
// lands would bounce everyone who just added playlists in step 2.
test("no bounce while the check is still in flight", () => {
  const goToVideosStep = jest.fn();
  renderLastStep(container, {
    numEnabledVideos: 0,
    refreshPageLoadData: () => new Promise(() => {}),
    goToVideosStep,
  });
  expect(goToVideosStep).not.toHaveBeenCalled();
});

test("no bounce when the check itself failed", async () => {
  const goToVideosStep = jest.fn();
  await act(async () => {
    ReactDOM.render(
      <StartPlayingTabView
        numEnabledVideos={0}
        refreshPageLoadData={() => Promise.resolve(false)}
        goToVideosStep={goToVideosStep}
      />,
      container
    );
  });
  expect(goToVideosStep).not.toHaveBeenCalled();
});

test("no bounce when the check rejected", async () => {
  const goToVideosStep = jest.fn();
  await act(async () => {
    ReactDOM.render(
      <StartPlayingTabView
        numEnabledVideos={0}
        refreshPageLoadData={() => Promise.reject(new Error("offline"))}
        goToVideosStep={goToVideosStep}
      />,
      container
    );
  });
  expect(goToVideosStep).not.toHaveBeenCalled();
});

// The repair page is the wizard's videos step behind the same PIN /settings
// requires (PlaylistsFloorGate is the shared control): same floor, same
// server tally, different exit. The gate renders inline here, so unlocking
// means typing the code.
const renderRepair = async (container, playableTotal) => {
  global.fetch = jest.fn(() =>
    Promise.resolve({
      ok: true,
      json: () =>
        Promise.resolve({ playlists: [], playable_total: playableTotal }),
    })
  );
  await act(async () => {
    ReactDOM.render(
      // The router context is for PinView's useParams: the inline gate never
      // reads a route param, but the hook still needs a router above it.
      <MemoryRouter>
        <VideosRepairView token="t" apiUrl="/api/v1" user={SET_UP_USER} />
      </MemoryRouter>,
      container
    );
  });
};

const repairButton = (container) =>
  [...container.querySelectorAll("button")].find(
    (b) => b.textContent === "Start Playing!"
  );

const typePin = async (container, pin) => {
  const input = container.querySelector("input.mock-pin");
  const setValue = Object.getOwnPropertyDescriptor(
    window.HTMLInputElement.prototype,
    "value"
  ).set;
  await act(async () => {
    setValue.call(input, pin);
    input.dispatchEvent(new Event("input", { bubbles: true }));
  });
};

afterEach(() => {
  ClearSessionPin();
  delete global.fetch;
});

test("the repair page asks for the PIN under its own heading", async () => {
  await renderRepair(container, 0);
  expect(container.querySelector("h2").textContent).toBe(
    "Add videos to keep playing!"
  );
  expect(container.textContent).toMatch(/Enter your four digit PIN code/);
  expect(repairButton(container)).toBeUndefined();
});

// This page stands in for /play, where the device changes hands, so it never
// reads the session: the code has to be typed here. Dropping that leftover
// session is no longer this view's job — one rule in pin.js clears it for any
// takeover, and pin_gate.test.js pins that half.
test("a leftover adult session does not unlock the page", async () => {
  SetSessionPin(SET_UP_USER.pin);
  await renderRepair(container, 0);
  expect(container.textContent).toMatch(/Enter your four digit PIN code/);
  expect(repairButton(container)).toBeUndefined();
});

test("a wrong PIN leaves the page locked", async () => {
  await renderRepair(container, 3);
  await typePin(container, "9999");
  expect(repairButton(container)).toBeUndefined();
});

test("the repair page gates Start Playing on the video floor", async () => {
  await renderRepair(container, 2);
  await typePin(container, SET_UP_USER.pin);
  expect(repairButton(container).className).toMatch(/error/);
});

test("the repair page releases the gate at the floor", async () => {
  await renderRepair(container, 3);
  await typePin(container, SET_UP_USER.pin);
  expect(repairButton(container).className).not.toMatch(/error/);
});

// The wizard's PIN step is the one caller that authors a code instead of
// proving one, so it is the one place PinView's verification is switched off.
// Untested until now, and the failure it guards against is total: a step that
// verified against a PIN the account does not have yet would reject every
// entry, and a first-run parent could never finish onboarding.
const renderPinTab = (container, user, advanceSetup = () => {}) =>
  act(() => {
    ReactDOM.render(
      <PinTabView
        token="t"
        apiUrl="/api/v1"
        user={user}
        advanceSetup={advanceSetup}
      />,
      container
    );
  });

const continueButton = (container) => container.querySelector("button.submit");

test("the wizard's PIN step accepts a code the account does not have yet", async () => {
  global.fetch = jest.fn(() => Promise.resolve({ json: () => ({}) }));
  const user = { auth0_id: "auth0|abc", pin: "" };
  renderPinTab(container, user);

  expect(continueButton(container).className).toMatch(/error/);

  await typePin(container, "8080");
  expect(continueButton(container).className).not.toMatch(/error/);
  expect(GetSessionPin()).toBe("8080");
});

test("the wizard's PIN step writes the authored code and advances", async () => {
  global.fetch = jest.fn(() => Promise.resolve({ json: () => ({}) }));
  const advanceSetup = jest.fn();
  const user = { auth0_id: "auth0|abc", pin: "" };
  renderPinTab(container, user, advanceSetup);

  await typePin(container, "8080");
  await act(async () => {
    continueButton(container).dispatchEvent(
      new MouseEvent("click", { bubbles: true })
    );
  });

  expect(user.pin).toBe("8080");
  expect(global.fetch).toHaveBeenCalled();
  expect(advanceSetup).toHaveBeenCalled();
});

test("the wizard's PIN step does not rewrite an unchanged prefilled code", async () => {
  global.fetch = jest.fn(() => Promise.resolve({ json: () => ({}) }));
  const advanceSetup = jest.fn();
  SetSessionPin("1234");
  renderPinTab(container, { auth0_id: "auth0|abc", pin: "1234" }, advanceSetup);

  await act(async () => {
    continueButton(container).dispatchEvent(
      new MouseEvent("click", { bubbles: true })
    );
  });

  expect(global.fetch).not.toHaveBeenCalled();
  expect(advanceSetup).toHaveBeenCalled();
});
