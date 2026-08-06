import React from "react";
import ReactDOM from "react-dom";
import { act } from "react-dom/test-utils";

import {
  PROBLEM_TYPE_GROUPS,
  ProblemTypesSettingsView,
  applyToggleRules,
  errorCardTitle,
} from "./settings.js";
import { validateBitmap } from "./bitmap_validation.js";
import { ProblemTypes as T } from "./enums.generated.js";

// The toggle rules and the error placement are derived from
// PROBLEM_TYPE_GROUPS, so these tests are written over the taxonomy rather
// than over today's bit list: a new bit with a dependency the derivation
// can't see fails here instead of on a parent's screen (an orphaned child bit
// makes validateBitmap reject, and the screen then silently stops saving).

const ENTRIES = PROBLEM_TYPE_GROUPS.reduce(
  (all, group) => all.concat(group.entries),
  []
);
const ALL_BITS = ENTRIES.reduce((mask, entry) => mask | entry.bit, 0);

describe("applyToggleRules", () => {
  it("leaves a valid bitmap valid when any single bit is switched off", () => {
    expect(validateBitmap(ALL_BITS).valid).toBe(true);
    ENTRIES.forEach((entry) => {
      const off = applyToggleRules(ALL_BITS, entry.bit, false);
      expect(off & entry.bit).toBe(0);
      expect(validateBitmap(off).valid).toBe(true);
    });
  });

  it("leaves a valid bitmap valid when any single bit is switched on", () => {
    ENTRIES.forEach((entry) => {
      const on = applyToggleRules(T.ADDITION, entry.bit, true);
      expect(on & entry.bit).toBe(entry.bit);
      expect(validateBitmap(on).valid).toBe(true);
    });
  });

  it("clears every dependent when its parent goes off", () => {
    ENTRIES.filter((entry) => entry.dependsOn != null).forEach((entry) => {
      expect(
        applyToggleRules(ALL_BITS, entry.dependsOn, false) & entry.bit
      ).toBe(0);
    });
  });

  it("pulls MEDIUM in behind the bits that need it, and back out with it", () => {
    expect(applyToggleRules(T.ADDITION, T.LARGE_NUMBERS, true)).toBe(
      T.ADDITION | T.LARGE_NUMBERS | T.MEDIUM_NUMBERS
    );
    expect(
      applyToggleRules(T.MULTIPLICATION, T.PERCENTAGES, true) & T.MEDIUM_NUMBERS
    ).toBe(T.MEDIUM_NUMBERS);
    const noMedium = applyToggleRules(ALL_BITS, T.MEDIUM_NUMBERS, false);
    expect(noMedium & (T.LARGE_NUMBERS | T.PERCENTAGES)).toBe(0);
  });

  it("clears a dependent's own dependents too", () => {
    // MULTIPLICATION -> PERCENTAGES -> MEDIUM: dropping MULTIPLICATION drops
    // PERCENTAGES, and anything hanging off PERCENTAGES goes with it.
    const noMul = applyToggleRules(ALL_BITS, T.MULTIPLICATION, false);
    expect(noMul & T.PERCENTAGES).toBe(0);
    expect(validateBitmap(noMul).valid).toBe(true);
  });
});

describe("dependent chips", () => {
  // The view seeds its state from the bitmap prop once, so each case mounts
  // fresh rather than re-rendering the same tree.
  const renderAt = (container, bitmap) =>
    act(() => {
      ReactDOM.unmountComponentAtNode(container);
      ReactDOM.render(
        <ProblemTypesSettingsView
          token="t"
          apiUrl="/api/v1"
          user={{ id: 1 }}
          settings={{ user_id: 1, problem_type_bitmap: bitmap }}
          errCallback={() => {}}
        />,
        container
      );
    });

  it("gates each dependent on its parent, and marks the parent row", () => {
    const container = document.createElement("div");
    document.body.appendChild(container);
    ENTRIES.filter((entry) => entry.dependsOn != null).forEach((entry) => {
      const chip = () => container.querySelector("#pt-" + entry.bit);
      renderAt(container, T.ADDITION);
      expect(chip().disabled).toBe(true);
      expect(
        container.querySelector("#pt-" + entry.dependsOn).closest("li")
          .className
      ).toBe("has-dep");

      renderAt(container, T.ADDITION | entry.dependsOn);
      expect(chip().disabled).toBe(false);
    });
    document.body.removeChild(container);
  });
});

describe("errorCardTitle", () => {
  it("anchors every error a bitmap can produce to a card", () => {
    const CARDS = PROBLEM_TYPE_GROUPS.map((group) => group.title);
    // One bitmap per rule, each tripping the error it names.
    [
      0,
      T.ADDITION | T.LARGE_NUMBERS,
      T.ADDITION | T.MISMATCHED_DENOMINATORS,
      T.ADDITION | T.PEMDAS,
      T.ADDITION | T.PERCENTAGES,
      T.MULTIPLICATION | T.PERCENTAGES,
    ].forEach((bitmap) => {
      const result = validateBitmap(bitmap);
      expect(result.valid).toBe(false);
      result.errors.forEach((err) => {
        expect(CARDS).toContain(errorCardTitle(err));
      });
    });
  });

  it("still names a card for an error that names no bits", () => {
    // A new error code that forgets its `bits` must land somewhere: the
    // screen refuses to save while any error stands, and there is no error
    // boundary above this view to catch a throw.
    expect(PROBLEM_TYPE_GROUPS.map((group) => group.title)).toContain(
      errorCardTitle({ code: "NEW_RULE", message: "…" })
    );
  });

  it("keeps each error on the card its bits live in", () => {
    const cardFor = (code, bitmap) =>
      errorCardTitle(
        validateBitmap(bitmap).errors.find((err) => err.code === code)
      );
    expect(cardFor("NO_CORE_OP", 0)).toBe("Operations");
    expect(cardFor("LARGE_REQUIRES_MEDIUM", T.ADDITION | T.LARGE_NUMBERS)).toBe(
      "Number size"
    );
    expect(
      cardFor(
        "MISMATCHED_REQUIRES_FRACTIONS",
        T.ADDITION | T.MISMATCHED_DENOMINATORS
      )
    ).toBe("Number types");
    expect(cardFor("PEMDAS_REQUIRES_CHAINED", T.ADDITION | T.PEMDAS)).toBe(
      "Problem format"
    );
    expect(
      cardFor("PERCENTAGES_REQUIRE_MULTIPLICATION", T.ADDITION | T.PERCENTAGES)
    ).toBe("Operations");
    expect(
      cardFor("PERCENTAGES_REQUIRE_MEDIUM", T.MULTIPLICATION | T.PERCENTAGES)
    ).toBe("Operations");
  });
});
