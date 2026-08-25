import React, { act } from "react";
import { renderInto, unmountFrom } from "./test_dom.js";

import { PinConfirmModal } from "./pin_confirm_modal.js";

// Both flows that stand between a kid and an adult-only action now share this
// component, so its gate is pinned once here: confirm is unreachable without a
// full PIN, the entered PIN reaches the caller, and an in-flight submit can't be
// dismissed out from under the request.

// react-pin-input drives its per-digit focus through real timers, which jsdom
// can't satisfy. Stand in a single input that reports the same thing the widget
// reports (the concatenated value).
vi.mock("react-pin-input", async () => {
  const { createElement } = await vi.importActual("react");
  return {
    __esModule: true,
    default: ({ onChange, focus }) =>
      createElement("input", {
        className: "mock-pin",
        autoFocus: focus,
        onChange: (e) => onChange(e.target.value),
      }),
  };
});

const click = (el) =>
  act(() => {
    el.dispatchEvent(new MouseEvent("click", { bubbles: true }));
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

describe("PinConfirmModal", () => {
  let container;
  let onConfirm;
  let onCancel;

  const render = (props = {}) =>
    act(() => {
      renderInto(
        <PinConfirmModal
          title="Confirm action"
          copy="Enter your PIN."
          confirmLabel="Confirm"
          submittingLabel="Confirming…"
          onConfirm={onConfirm}
          onCancel={onCancel}
          {...props}
        />,
        container,
      );
    });

  const confirmButton = () =>
    container.querySelectorAll(".pin-confirm-modal-actions button")[1];
  const cancelButton = () =>
    container.querySelectorAll(".pin-confirm-modal-actions button")[0];

  beforeEach(() => {
    container = document.createElement("div");
    document.body.appendChild(container);
    onConfirm = vi.fn();
    onCancel = vi.fn();
  });

  afterEach(() => {
    act(() => {
      unmountFrom(container);
    });
    container.remove();
  });

  it("keeps confirm disabled until a full PIN is entered", () => {
    render();
    expect(confirmButton().disabled).toBe(true);

    typePin(container, "12");
    expect(confirmButton().disabled).toBe(true);

    typePin(container, "1234");
    expect(confirmButton().disabled).toBe(false);
  });

  it("puts the caret in the PIN on open", () => {
    render();
    expect(document.activeElement).toBe(
      container.querySelector("input.mock-pin"),
    );
  });

  it("hands the entered PIN to onConfirm", () => {
    render();
    typePin(container, "4321");
    click(confirmButton());
    expect(onConfirm).toHaveBeenCalledWith("4321");
  });

  it("cancels on the cancel button and on an outside click", () => {
    render();
    click(cancelButton());
    expect(onCancel).toHaveBeenCalledTimes(1);

    click(container.querySelector(".pin-confirm-modal-overlay"));
    expect(onCancel).toHaveBeenCalledTimes(2);
  });

  it("does not cancel from a click inside the card", () => {
    render();
    click(container.querySelector(".pin-confirm-modal-copy"));
    expect(onCancel).not.toHaveBeenCalled();
  });

  it("blocks dismissal and re-submission while a submit is in flight", () => {
    render({ submitting: true });
    typePin(container, "1234");

    expect(confirmButton().disabled).toBe(true);
    expect(confirmButton().textContent).toBe("Confirming…");

    click(cancelButton());
    click(container.querySelector(".pin-confirm-modal-overlay"));
    expect(onCancel).not.toHaveBeenCalled();
  });

  it("renders extra fields as children and shows the error line", () => {
    render({
      error: "Incorrect PIN",
      children: <textarea id="extra-field" />,
    });
    expect(container.querySelector("#extra-field")).not.toBeNull();
    expect(
      container.querySelector(".pin-confirm-modal-error").textContent,
    ).toBe("Incorrect PIN");
  });
});
