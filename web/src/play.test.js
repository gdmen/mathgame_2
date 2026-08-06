import React from "react";
import ReactDOM from "react-dom";
import { act } from "react-dom/test-utils";

import { PlayView } from "./play.js";

// `./conf` is generated from the backend conf (gen_frontend_conf.py), so it is
// gitignored and absent on a clean checkout — this is the first suite to pull
// in a module that reads it. Virtual, therefore, rather than a fixture: it
// stands in whether or not the real file exists. debug_quickplay is the field
// PlayView reads, and false is the shipped value; true makes it render null.
jest.mock("./conf", () => ({ debug_quickplay: false }), { virtual: true });

// /play 403s when the reward pool is below the floor. The answer to that is
// the repair page, which the takeover puts on the screen once it sees a short
// count — so PlayView's whole job here is to re-read the count. Pinned: that
// it does, that a healthy load doesn't, that a 403 landing after unmount is
// dropped rather than rewriting the payload of whatever took the screen, and
// that a 403 nothing resolves ends somewhere the kid can read instead of on a
// spinner that never stops.

const render = (container, props) =>
  act(() => {
    ReactDOM.render(
      <PlayView
        token="t"
        apiUrl="http://api"
        user={{ id: 7 }}
        postEvent={jest.fn()}
        interval={100000}
        {...props}
      />,
      container,
    );
  });

let container;
let refreshPageLoadData;
beforeEach(() => {
  container = document.createElement("div");
  document.body.appendChild(container);
  refreshPageLoadData = jest.fn(() => Promise.resolve(true));
});
afterEach(() => {
  ReactDOM.unmountComponentAtNode(container);
  container.remove();
  delete global.fetch;
});

test("re-reads the page load data when the pool is below the floor", async () => {
  global.fetch = jest.fn(() =>
    Promise.resolve({ ok: false, status: 403, text: () => "" }),
  );

  await act(async () => {
    render(container, { refreshPageLoadData });
  });

  expect(refreshPageLoadData).toHaveBeenCalledTimes(1);
});

// PlayView renders alone here, so nothing takes the screen away from it —
// which is exactly the case the message exists for: the refresh failed, or the
// 403 was never about the video pool.
test("says so when nothing resolves the 403", async () => {
  global.fetch = jest.fn(() =>
    Promise.resolve({ ok: false, status: 403, text: () => "" }),
  );

  await act(async () => {
    render(container, { refreshPageLoadData: () => Promise.resolve(false) });
  });

  expect(container.querySelector(".content-loading")).toBeNull();
  expect(container.textContent).toMatch(/We couldn't start the game/);
  expect(container.querySelector("a").getAttribute("href")).toBe("/settings");
});

// The refresh the 403 asks for hands the app a brand new user object, so an
// effect keyed on that object would answer its own 403 with another /play
// request, forever.
test("does not re-ask /play when the refresh hands down a new user object", async () => {
  global.fetch = jest.fn(() =>
    Promise.resolve({ ok: false, status: 403, text: () => "" }),
  );

  await act(async () => {
    render(container, { refreshPageLoadData });
  });
  await act(async () => {
    render(container, { refreshPageLoadData });
  });

  expect(global.fetch).toHaveBeenCalledTimes(1);
  expect(refreshPageLoadData).toHaveBeenCalledTimes(1);
});

test("leaves the page load data alone on a healthy load", async () => {
  global.fetch = jest.fn(() =>
    Promise.resolve({
      ok: true,
      status: 200,
      text: () =>
        JSON.stringify({
          gamestate: { solved: 0, target: 5, video_id: 1, problem_id: 1 },
          problem: { id: 1, expression: "1+1", answer: "2" },
          video: { id: 1 },
        }),
    }),
  );

  await act(async () => {
    render(container, { refreshPageLoadData });
  });

  expect(refreshPageLoadData).not.toHaveBeenCalled();
});

test("drops a 403 that lands after unmount", async () => {
  let respond;
  global.fetch = jest.fn(
    () =>
      new Promise((resolve) => {
        respond = () => resolve({ ok: false, status: 403, text: () => "" });
      }),
  );

  render(container, { refreshPageLoadData });
  act(() => {
    ReactDOM.unmountComponentAtNode(container);
  });
  await act(async () => {
    respond();
  });

  expect(refreshPageLoadData).not.toHaveBeenCalled();
});
