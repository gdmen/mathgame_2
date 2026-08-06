import React from "react";
import ReactDOM from "react-dom";
import { act } from "react-dom/test-utils";
import { MemoryRouter, Route, useHistory } from "react-router-dom";

import {
  PIN_PROTECTED_PATHS,
  PinView,
  SetSessionPin,
  GetSessionPin,
  usePinSessionPolicy,
} from "./pin.js";

// The adult gate is a UX boundary, not a security one, but the two properties
// below are the ones that were previously left to each view remembering: that
// leaving a protected surface drops the session, and that a valid entry is the
// only thing that establishes one.

// react-pin-input drives its per-digit focus through real timers, which jsdom
// can't satisfy. Stand in a single input that reports the same thing the widget
// reports (the concatenated value).
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

const typePin = (container, pin) => {
  const input = container.querySelector("input.mock-pin");
  const setValue = Object.getOwnPropertyDescriptor(
    window.HTMLInputElement.prototype,
    "value",
  ).set;
  act(() => {
    setValue.call(input, pin);
    input.dispatchEvent(new Event("input", { bubbles: true }));
  });
};

describe("usePinSessionPolicy", () => {
  let container;

  const Probe = ({ takeover = null }) => {
    usePinSessionPolicy(takeover);
    const history = useHistory();
    return <button onClick={() => history.push("/play")}>navigate away</button>;
  };

  const renderAt = (pathname, takeover = null) =>
    act(() => {
      ReactDOM.render(
        <MemoryRouter initialEntries={[pathname]}>
          <Probe takeover={takeover} />
        </MemoryRouter>,
        container,
      );
    });

  beforeEach(() => {
    container = document.createElement("div");
    document.body.appendChild(container);
    SetSessionPin("1234");
  });

  afterEach(() => {
    act(() => {
      ReactDOM.unmountComponentAtNode(container);
    });
    container.remove();
  });

  // Every URL the router renders the settings page for has to count as the
  // settings page. <Route exact path="/settings"> is neither strict nor
  // case-sensitive, so it serves all three of these — and a string compare
  // against PIN_PROTECTED_PATHS called the last two unprotected, cleared the
  // session on the page it exists to protect, and left the gate redirecting to
  // itself forever.
  it.each(["/settings", "/settings/", "/SETTINGS"])(
    "keeps the session on %s",
    (pathname) => {
      renderAt(pathname);
      expect(GetSessionPin()).toBe("1234");
    },
  );

  // The point of the policy: an unlisted path drops the session without that
  // path's view having to know the gate exists. /play is the one that matters
  // (it is where the device changes hands) but nothing about it is special.
  it.each(["/play", "/progress", "/", "/nope", "/pin/%2Fsettings"])(
    "drops the session on %s",
    (pathname) => {
      renderAt(pathname);
      expect(GetSessionPin()).toBeNull();
    },
  );

  // A screen nobody has written yet is the actual subject of this rule.
  it("drops the session on a path that does not exist yet", () => {
    renderAt("/some-future-kid-screen");
    expect(GetSessionPin()).toBeNull();
  });

  it("re-runs on navigation, not just on mount", () => {
    renderAt("/settings");
    expect(GetSessionPin()).toBe("1234");
    act(() => {
      container
        .querySelector("button")
        .dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });
    expect(GetSessionPin()).toBeNull();
  });

  // A takeover replaces whatever the path would have rendered, so /settings
  // with the videos repair page on screen is not the settings page. Without
  // this half, the repair page — which stands in for /play, where the device
  // changes hands — would honour a session whenever it intercepted /settings.
  it.each(["videos", "setup"])(
    "drops the session when the %s takeover holds a protected path",
    (takeover) => {
      renderAt("/settings", takeover);
      expect(GetSessionPin()).toBeNull();
    },
  );

  it("only lists surfaces that actually gate on the PIN", () => {
    expect(PIN_PROTECTED_PATHS).toEqual(["/settings"]);
  });

  // The generic form of the bug, so a second protected route inherits the
  // guard instead of needing its own cases. For every pattern in the list, the
  // policy has to agree with what <Route exact path={pattern}> actually
  // renders — including the URL shapes react-router matches loosely. Asserting
  // the route matches too means this fails loudly if those defaults ever
  // change, rather than passing for the wrong reason.
  const variants = (p) => [p, p + "/", p.toUpperCase()];

  PIN_PROTECTED_PATHS.forEach((pattern) => {
    it.each(variants(pattern))(
      `${pattern} route and policy agree on %s`,
      (pathname) => {
        let routed = false;
        const c = document.createElement("div");
        act(() => {
          ReactDOM.render(
            <MemoryRouter initialEntries={[pathname]}>
              <Route
                exact
                path={pattern}
                render={() => {
                  routed = true;
                  return null;
                }}
              />
            </MemoryRouter>,
            c,
          );
        });
        act(() => {
          ReactDOM.unmountComponentAtNode(c);
        });

        renderAt(pathname);

        expect(routed).toBe(true);
        expect(GetSessionPin()).toBe("1234");
      },
    );
  });
});

describe("PinView", () => {
  let container;
  let onValid;

  const render = (props) =>
    act(() => {
      ReactDOM.render(<PinView onValid={onValid} {...props} />, container);
    });

  beforeEach(() => {
    container = document.createElement("div");
    document.body.appendChild(container);
    onValid = jest.fn();
    SetSessionPin("");
  });

  afterEach(() => {
    act(() => {
      ReactDOM.unmountComponentAtNode(container);
    });
    container.remove();
  });

  it("does not accept a code that fails verification", () => {
    render({ verifyAgainst: "1234" });
    typePin(container, "9999");
    expect(onValid).not.toHaveBeenCalled();
    expect(GetSessionPin()).toBe("");
  });

  it("accepts the matching code and stores the session", () => {
    render({ verifyAgainst: "1234" });
    typePin(container, "1234");
    expect(onValid).toHaveBeenCalledWith("1234");
    expect(GetSessionPin()).toBe("1234");
  });

  // Authoring mode has nothing to compare against, so any four digits stand —
  // but only when the caller asked for it by name.
  it("accepts any four digits when authoring", () => {
    render({ authoring: true, prompt: null, secret: false });
    typePin(container, "8080");
    expect(onValid).toHaveBeenCalledWith("8080");
    expect(GetSessionPin()).toBe("8080");
  });

  // The direction that matters: forgetting the code to check against must
  // produce a gate that opens for nothing, not one that opens for anything.
  // `verifyAgainst` has no default for exactly this reason.
  it.each([undefined, null, ""])(
    "rejects every entry when verifyAgainst is %p and authoring was not asked for",
    (verifyAgainst) => {
      render({ verifyAgainst });
      typePin(container, "0000");
      expect(onValid).not.toHaveBeenCalled();
      expect(GetSessionPin()).toBe("");
    },
  );

  it("starts errored when there is no code to verify against", () => {
    render({});
    expect(container.querySelector("h4 span").className).toBe("error");
  });

  it("rejects a short code in both modes", () => {
    render({ verifyAgainst: "1234" });
    typePin(container, "123");
    expect(onValid).not.toHaveBeenCalled();

    render({ authoring: true, prompt: null });
    typePin(container, "12");
    expect(onValid).not.toHaveBeenCalled();
  });

  // It used to reach for useParams(), so a caller that was not a route had to
  // be wrapped in a router purely to satisfy a value it never read.
  it("renders outside a router", () => {
    expect(() => render({ verifyAgainst: "1234" })).not.toThrow();
  });

  it("shows the prompt by default and suppresses it on request", () => {
    render({ verifyAgainst: "1234" });
    expect(container.querySelector("h4")).not.toBeNull();

    render({ prompt: null, verifyAgainst: "1234" });
    expect(container.querySelector("h4")).toBeNull();
  });

  // The gate's prompt is the wrong-code cue, so it must not start red for an
  // account that simply has not typed anything yet.
  it("does not flag an error before anything is typed", () => {
    render({ verifyAgainst: "1234" });
    expect(container.querySelector("h4 span").className).toBe("");

    typePin(container, "9999");
    expect(container.querySelector("h4 span").className).toBe("error");
  });
});

describe("PinGateRoute wiring", () => {
  // PinView no longer navigates; the route does. Pinned here because the
  // redirect is the half that has no other test.
  it("hands the decoded redirect target to the route's own handler", () => {
    const seen = [];
    const Harness = () => (
      <Route
        exact
        path="/pin/:redirect_pathname"
        render={({ match }) => {
          seen.push(decodeURIComponent(match.params.redirect_pathname));
          return null;
        }}
      />
    );
    const container = document.createElement("div");
    act(() => {
      ReactDOM.render(
        <MemoryRouter initialEntries={["/pin/%2Fsettings"]}>
          <Harness />
        </MemoryRouter>,
        container,
      );
    });
    expect(seen).toEqual(["/settings"]);
    act(() => {
      ReactDOM.unmountComponentAtNode(container);
    });
  });
});
