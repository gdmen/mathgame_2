import React from "react";
import ReactDOM from "react-dom";
import { act } from "react-dom/test-utils";

import { ProblemView } from "./problem.js";

// "Time on problem" is accrued by a sticky event the reporter re-POSTs on every
// tick, so whether it is in the set has to track exactly what the kid is
// looking at. These pin that lifecycle: it used to be maintained by clearing
// the whole set on every render and re-adding mid-render, which was invisible
// to any test and re-ran constantly.
const makeReporter = () => {
  const events = new Set();
  return {
    events,
    add: (e) => events.add(e),
    remove: (e) => events.delete(e),
    postEvent: () => Promise.resolve(null),
  };
};

const GAMESTATE = { problem_id: 7, solved: 1, target: 3 };
const LATEX = "<span>4 + 3</span>";

const renderProblem = (container, reporter, gamestate = GAMESTATE) =>
  act(() => {
    ReactDOM.render(
      <ProblemView
        gamestate={gamestate}
        latex={LATEX}
        eventReporter={reporter}
        interval={5000}
      />,
      container
    );
  });

describe("ProblemView working_on_problem reporting", () => {
  let container;

  beforeEach(() => {
    container = document.createElement("div");
    document.body.appendChild(container);
  });

  afterEach(() => {
    act(() => {
      ReactDOM.unmountComponentAtNode(container);
    });
    container.remove();
  });

  it("accrues time while an unanswered problem is on screen", () => {
    const reporter = makeReporter();
    renderProblem(container, reporter);
    expect(reporter.events.has("working_on_problem")).toBe(true);
  });

  it("stops accruing when the view goes away (the reward video)", () => {
    const reporter = makeReporter();
    renderProblem(container, reporter);
    act(() => {
      ReactDOM.unmountComponentAtNode(container);
    });
    expect(reporter.events.has("working_on_problem")).toBe(false);
  });

  it("stops accruing while a submitted answer is in flight", () => {
    const reporter = makeReporter();
    renderProblem(container, reporter);

    // Typing and submitting must be separate commits: batched into one, the
    // click handler would still close over the empty answer and never submit.
    act(() => {
      const input = container.querySelector("#problem-answer-input");
      const setValue = Object.getOwnPropertyDescriptor(
        window.HTMLInputElement.prototype,
        "value"
      ).set;
      setValue.call(input, "7");
      input.dispatchEvent(new Event("input", { bubbles: true }));
    });
    act(() => {
      container
        .querySelector("button")
        .dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });

    expect(container.querySelector("#problem").className).toContain(
      "submitting"
    );
    expect(reporter.events.has("working_on_problem")).toBe(false);
  });

  it("resumes accruing on the next problem", () => {
    const reporter = makeReporter();
    renderProblem(container, reporter);
    reporter.events.clear();
    renderProblem(container, reporter, { ...GAMESTATE, problem_id: 8 });
    expect(reporter.events.has("working_on_problem")).toBe(true);
  });
});
