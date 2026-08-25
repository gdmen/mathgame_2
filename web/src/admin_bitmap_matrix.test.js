import React, { act } from "react";
import { renderInto, unmountFrom } from "./test_dom.js";

import { BitmapMatrixView } from "./admin_bitmap_matrix.js";

// The matrix is the one view built on the virtualized grid, and the grid's
// cell-rendering contract is the part a react-window upgrade moves. Pinned
// here: that a report renders a cell per (row, bucket), including the two cells
// that take their own early return — a column past the row's ceiling, and an
// over-ceiling cell.
const REPORT = {
  axis_lo: 1,
  axis_hi: 3,
  pool_max: 10,
  rows: [
    {
      bitmap: 1,
      bits: ["ADD"],
      ceil: 5,
      cells: [
        { e: "1+1", a: "2", d: 1, p: 5 },
        { e: "2+2", a: "4", d: 2, p: 3 },
        { o: true, p: 2 },
      ],
    },
    // Two cells against a three-column axis, so the last column is past this
    // row's own end.
    {
      bitmap: 2,
      bits: ["SUB"],
      ceil: 5,
      cells: [
        { e: "3-1", a: "2", d: 1, p: 1 },
        { e: "", a: "", d: 0, p: 0 },
      ],
    },
  ],
};

let container;

beforeEach(() => {
  global.fetch = vi.fn(() =>
    Promise.resolve({
      ok: true,
      headers: { get: (name) => (name === "X-Has-Report" ? "true" : "") },
      json: () => Promise.resolve(REPORT),
    }),
  );
  container = document.createElement("div");
  document.body.appendChild(container);
});

afterEach(() => {
  act(() => {
    unmountFrom(container);
  });
  container.remove();
  delete global.fetch;
});

const render = () =>
  act(async () => {
    renderInto(
      <BitmapMatrixView token="t" apiUrl="/api/v1" user={{ id: 1 }} />,
      container,
    );
  });

test("renders one cell per row and difficulty bucket", async () => {
  await render();
  expect(container.querySelectorAll(".bm-cell")).toHaveLength(6);
  expect(container.querySelectorAll(".bm-cell-over")).toHaveLength(1);
  expect(container.querySelectorAll(".bm-cell-blank")).toHaveLength(1);
});

test("a cell carries its expression and pool count", async () => {
  await render();
  expect(container.textContent).toMatch(/pool 5/);
  expect(container.querySelectorAll("button.bm-reroll").length).toBeGreaterThan(
    0,
  );
});
