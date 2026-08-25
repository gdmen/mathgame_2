import React, { act } from "react";
import { renderInto, unmountFrom } from "./test_dom.js";

import { DeleteAccountView } from "./settings.js";
import { SetSessionPin, GetSessionPin } from "./pin.js";

// The only irreversible action a parent can take from the app, so the pieces
// that stand between a stray tap and a deleted account are pinned here: the
// confirm button stays disabled until a full PIN is entered, a rejected PIN
// leaves the session intact, and only a 204 tears the session down.
// vi.hoisted because the mock factory runs before this module's own bindings
// are initialised.
const mockLogout = vi.hoisted(() => vi.fn());
vi.mock("@auth0/auth0-react", () => ({
  useAuth0: () => ({ logout: mockLogout }),
}));

// react-pin-input drives its per-digit focus through real timers, which jsdom
// can't satisfy. Stand in a single input that reports the same thing the widget
// reports (the concatenated value) so these tests exercise our state machine
// rather than the widget's.
vi.mock("react-pin-input", async () => {
  const { createElement } = await vi.importActual("react");
  return {
    __esModule: true,
    default: ({ onChange }) =>
      createElement("input", {
        className: "mock-pin",
        onChange: (e) => onChange(e.target.value),
      }),
  };
});

const USER = { id: 1, auth0_id: "auth0|abc", pin: "1234" };

const render = (container) =>
  act(() => {
    renderInto(
      <DeleteAccountView token="t" apiUrl="/api/v1" user={USER} />,
      container,
    );
  });

const openModal = (container) =>
  act(() => {
    container
      .querySelector("button.delete-account-open")
      .dispatchEvent(new MouseEvent("click", { bubbles: true }));
  });

const confirmButton = (container) =>
  container.querySelector("button.delete-account-confirm");

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

describe("DeleteAccountView", () => {
  let container;

  beforeEach(() => {
    container = document.createElement("div");
    document.body.appendChild(container);
    mockLogout.mockClear();
    SetSessionPin("1234");
  });

  afterEach(() => {
    act(() => {
      unmountFrom(container);
    });
    container.remove();
    delete global.fetch;
  });

  it("keeps the confirm button disabled until a full PIN is entered", () => {
    render(container);
    openModal(container);

    expect(confirmButton(container).disabled).toBe(true);

    typePin(container, "1234");
    expect(confirmButton(container).disabled).toBe(false);
  });

  it("surfaces a rejected PIN and leaves the session signed in", async () => {
    global.fetch = vi.fn(() => Promise.resolve({ status: 403 }));
    render(container);
    openModal(container);
    typePin(container, "9999");

    await act(async () => {
      confirmButton(container).dispatchEvent(
        new MouseEvent("click", { bubbles: true }),
      );
    });

    expect(
      container.querySelector(".pin-confirm-modal-error").textContent,
    ).toMatch(/Incorrect PIN/);
    expect(mockLogout).not.toHaveBeenCalled();
    expect(GetSessionPin()).toBe("1234");
  });

  it("clears the PIN session and logs out on a successful delete", async () => {
    global.fetch = vi.fn(() => Promise.resolve({ status: 204 }));
    render(container);
    openModal(container);
    typePin(container, "1234");

    await act(async () => {
      confirmButton(container).dispatchEvent(
        new MouseEvent("click", { bubbles: true }),
      );
    });

    const [url, opts] = global.fetch.mock.calls[0];
    expect(url).toBe("/api/v1/users/auth0%7Cabc");
    expect(opts.method).toBe("DELETE");
    expect(JSON.parse(opts.body)).toEqual({ pin: "1234" });
    expect(GetSessionPin()).toBe(null);
    expect(mockLogout).toHaveBeenCalled();
  });
});
