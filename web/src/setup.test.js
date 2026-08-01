import React from "react";
import ReactDOM from "react-dom";
import { act } from "react-dom/test-utils";

import { StartPlayingTabView, useSetupGate } from "./setup.js";

// The wizard is the only thing standing between a new account and an unplayable
// game, and its steps are what satisfy the gate that shows it — so the gate has
// to hold once it opens. Pinned here: it opens for an unfinished account, stays
// open after the account becomes complete mid-flow (step 4 refreshing the
// page-load data used to unmount the wizard mid-read), and never opens for an
// account that arrived complete.
const Probe = (props) => (useSetupGate(props) ? <i>setup</i> : <i>app</i>);

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
    onAdminPath: false,
  });
  expect(shown(container)).toBe("setup");
});

test("stays open once the wizard's own steps satisfy the gate", () => {
  render(container, {
    user: NEW_USER,
    settings: {},
    numEnabledVideos: 0,
    onAdminPath: false,
  });
  render(container, {
    user: SET_UP_USER,
    settings: {},
    numEnabledVideos: 12,
    onAdminPath: false,
  });
  expect(shown(container)).toBe("setup");
});

test("never opens for an account that is already set up", () => {
  render(container, {
    user: SET_UP_USER,
    settings: {},
    numEnabledVideos: 12,
    onAdminPath: false,
  });
  expect(shown(container)).toBe("app");
});

test("opens on too few playable videos even with a PIN set", () => {
  render(container, {
    user: SET_UP_USER,
    settings: {},
    numEnabledVideos: 2,
    onAdminPath: false,
  });
  expect(shown(container)).toBe("setup");
});

test("stays shut on an admin path with setup unfinished", () => {
  render(container, {
    user: NEW_USER,
    settings: {},
    numEnabledVideos: 0,
    onAdminPath: true,
  });
  expect(shown(container)).toBe("app");
});

test("stays shut before the page-load data arrives", () => {
  render(container, {
    user: null,
    settings: null,
    numEnabledVideos: null,
    onAdminPath: false,
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
