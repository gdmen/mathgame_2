import React, {
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { FixedSizeGrid } from "react-window";

import {
  renderMath,
  useAuthHeaders,
  usePollWhileComputing,
} from "./admin_common.js";
import "./admin_bitmap_matrix.scss";

// Grid geometry (px). Cells are wide enough for a small rendered expression;
// the left column holds the bitmap's feature list, the top strip the difficulty
// axis. FixedSizeGrid has no native sticky row/column, so the header and
// row-label strips are rendered separately and translated in lockstep with the
// grid's scroll offset (see onScroll below).
const CELL_W = 128;
const CELL_H = 78;
const LABEL_W = 220;
const HEADER_H = 28;
const GRID_H = 640;

// TARGET_WINDOW mirrors the generator's selection epsilon: a cell whose built
// difficulty lands more than this from its bucket is off-target — the envelope
// couldn't reach that bucket (e.g. a division-only bitmap has no problem below
// d≈7, so its d=3 cell shows the closest ~d=8 problem instead). Flag it so the
// example isn't misread as an on-target example for that difficulty.
const TARGET_WINDOW = 1.5;

const fmt1 = (n) => (n == null ? "—" : Number(n).toFixed(1));

// rowUsage sums the live pool counts across a row's buckets (the default sort
// key). rowComplexity is the enabled-bit count (the alternate sort key).
const rowUsage = (r) => r.cells.reduce((s, c) => s + (c.p || 0), 0);
const rowComplexity = (r) => r.bits.length;

// makeQuantile returns q(poolCount) → [0,1], the fraction of nonzero pool cells
// at or below that count. Rank-based, so the heavy-tailed pool distribution
// maps onto an even color spread (a few megacells don't wash everything red).
const makeQuantile = (rows) => {
  const vals = [];
  rows.forEach((r) => r.cells.forEach((c) => c.p > 0 && vals.push(c.p)));
  vals.sort((a, b) => a - b);
  return (v) => {
    if (v <= 0 || vals.length === 0) {
      return 0;
    }
    // Count of values <= v (upper-bound binary search).
    let lo = 0;
    let hi = vals.length;
    while (lo < hi) {
      const mid = (lo + hi) >> 1;
      if (vals[mid] <= v) {
        lo = mid + 1;
      } else {
        hi = mid;
      }
    }
    return lo / vals.length;
  };
};

// heatColor maps a quantile t∈[0,1] to red (zero / cold) → green (high / hot).
const heatColor = (t) => `hsl(${Math.round(120 * t)}, 65%, ${88 - 30 * t}%)`;

const BitmapMatrixView = ({ token, apiUrl, user }) => {
  const [data, setData] = useState(null);
  const [meta, setMeta] = useState({ hasReport: false, computing: false });
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [sortBy, setSortBy] = useState("usage");
  // Per-cell reroll results keyed "bitmap:bucket"; overlaid on the cached cell.
  const [overrides, setOverrides] = useState({});

  const authHeaders = useAuthHeaders(token);

  const fetchReport = useCallback(async () => {
    if (!token || !apiUrl || !user) {
      return;
    }
    try {
      const res = await fetch(apiUrl + "/admin/bitmap-matrix", {
        method: "GET",
        headers: authHeaders(),
      });
      if (!res.ok) {
        setError("Could not load the bitmap matrix");
        return;
      }
      const hasReport = res.headers.get("X-Has-Report") === "true";
      setMeta({
        hasReport,
        computing: res.headers.get("X-Computing") === "true",
        done: Number(res.headers.get("X-Compute-Done") || 0),
        total: Number(res.headers.get("X-Compute-Total") || 0),
        computedAt: res.headers.get("X-Computed-At") || "",
      });
      // The body (when present) is the gzipped report; the browser decompresses
      // it transparently, so res.json() yields the payload directly. Clear any
      // prior report if the cache is gone, so a stale grid never lingers.
      setData(hasReport ? await res.json() : null);
      setError(null);
    } catch (e) {
      setError(e.message || "Could not load the bitmap matrix");
    } finally {
      setLoading(false);
    }
  }, [token, apiUrl, user, authHeaders]);

  useEffect(() => {
    fetchReport();
  }, [fetchReport]);

  // While a rebuild is running, poll so the report (and progress) refresh.
  usePollWhileComputing(meta.computing, fetchReport);

  const recompute = async () => {
    try {
      await fetch(apiUrl + "/admin/bitmap-matrix/recompute", {
        method: "POST",
        headers: authHeaders(),
      });
    } catch (e) {
      // The next fetch reflects the real state.
    }
    setMeta((m) => ({ ...m, computing: true }));
    fetchReport();
  };

  const reroll = async (bitmap, bucket) => {
    try {
      const res = await fetch(
        apiUrl +
          "/admin/bitmap-matrix/cell?bitmap=" +
          bitmap +
          "&bucket=" +
          bucket,
        { method: "GET", headers: authHeaders() }
      );
      if (!res.ok) {
        return;
      }
      const cell = await res.json();
      setOverrides((o) => ({ ...o, [bitmap + ":" + bucket]: cell }));
    } catch (e) {
      // Leave the existing cell in place on failure.
    }
  };

  const axisLo = data ? data.axis_lo : 0;
  const axisHi = data ? data.axis_hi : 0;
  const numCols = data ? axisHi - axisLo + 1 : 0;

  const sortedRows = useMemo(() => {
    if (!data) {
      return [];
    }
    const rows = data.rows.slice();
    if (sortBy === "complexity") {
      rows.sort(
        (a, b) => rowComplexity(a) - rowComplexity(b) || a.bitmap - b.bitmap
      );
    } else {
      rows.sort((a, b) => rowUsage(b) - rowUsage(a) || a.bitmap - b.bitmap);
    }
    return rows;
  }, [data, sortBy]);

  const quantile = useMemo(
    () => (data ? makeQuantile(data.rows) : () => 0),
    [data]
  );

  // Scroll-sync the sticky header and row-label strips to the grid body.
  const headerRef = useRef(null);
  const rowLabelRef = useRef(null);
  const onScroll = useCallback(({ scrollLeft, scrollTop }) => {
    if (headerRef.current) {
      headerRef.current.style.transform = `translateX(${-scrollLeft}px)`;
    }
    if (rowLabelRef.current) {
      rowLabelRef.current.style.transform = `translateY(${-scrollTop}px)`;
    }
  }, []);

  // Measure the available width so the grid fills the viewport.
  const wrapRef = useRef(null);
  const [gridW, setGridW] = useState(900);
  useLayoutEffect(() => {
    const measure = () => {
      if (wrapRef.current) {
        setGridW(Math.max(320, wrapRef.current.clientWidth - LABEL_W));
      }
    };
    measure();
    window.addEventListener("resize", measure);
    return () => window.removeEventListener("resize", measure);
  }, [data]);

  const Cell = useCallback(
    ({ columnIndex, rowIndex, style }) => {
      const row = sortedRows[rowIndex];
      const bucket = axisLo + columnIndex;
      // The row runs only to its own ceiling (extended for over-ceiling pool
      // rows); columns past that are outside this bitmap's space.
      if (columnIndex >= row.cells.length) {
        return <div className="bm-cell bm-cell-blank" style={style} />;
      }
      let cell = row.cells[columnIndex];
      const ov = overrides[row.bitmap + ":" + bucket];
      if (ov) {
        cell = { ...cell, e: ov.e, a: ov.a, d: ov.d };
      }
      if (cell.o) {
        return (
          <div className="bm-cell bm-cell-over" style={style}>
            {cell.p > 0 && <span className="bm-pool">pool {cell.p}</span>}
          </div>
        );
      }
      const bg = heatColor(quantile(cell.p || 0));
      const offTarget =
        cell.e && Math.abs((cell.d || 0) - bucket) > TARGET_WINDOW;
      const title = offTarget
        ? `off-target: asked d≈${bucket}, closest the heuristic built was d=${fmt1(
            cell.d
          )}` + (cell.a ? ` · answer: ${cell.a}` : "")
        : cell.a
        ? "answer: " + cell.a
        : undefined;
      return (
        <div
          className={"bm-cell" + (offTarget ? " bm-cell-off" : "")}
          style={{ ...style, background: bg }}
          title={title}
        >
          {cell.e ? (
            <span className="bm-math">{renderMath(cell.e)}</span>
          ) : (
            <span className="bm-unbuildable">×</span>
          )}
          <span className="bm-meta">
            <span className={offTarget ? "bm-off" : undefined}>
              d={fmt1(cell.d)}
              {offTarget ? "≠" : ""}
            </span>
            <span className="bm-poolcount">pool {cell.p || 0}</span>
            <button
              className="bm-reroll"
              title="Regenerate this cell (does not change the cache)"
              onClick={() => reroll(row.bitmap, bucket)}
            >
              ↻
            </button>
          </span>
        </div>
      );
    },
    // reroll/quantile are stable-enough for the grid; overrides drives updates.
    [sortedRows, axisLo, overrides, quantile]
  );

  if (loading) {
    return <div className="content-loading"></div>;
  }
  if (error) {
    return (
      <div className="bm-page">
        <p className="bm-error">{error}</p>
      </div>
    );
  }

  const pct = meta.total > 0 ? Math.floor((100 * meta.done) / meta.total) : 0;

  return (
    <div className="bm-page">
      <h1>Bitmap × difficulty coverage matrix</h1>
      <p className="bm-hint">
        One live heuristic example per (bitmap, difficulty) cell across the
        entire valid non-WORD bitmap space (8,784 bitmaps), including cells the
        pool has never populated. Cell background heatmaps current pool usage —
        green = well-covered, red = zero — counted by exact bitmap match after
        the LARGE⇒MEDIUM normalization (<code>status=active</code> only; WORD
        rows out of scope). Greyed cells are above the bitmap's serving ceiling
        (not targetable). An amber outline (<span className="bm-off">d≠</span>)
        marks an off-target cell: the envelope couldn't reach that difficulty,
        so the example is the closest the generator managed. ↻ regenerates a
        cell live without touching the cache. The report is cached; rebuild with
        Recompute.
      </p>

      <div className="bm-control">
        <label>
          Sort:{" "}
          <select value={sortBy} onChange={(e) => setSortBy(e.target.value)}>
            <option value="usage">usage (desc)</option>
            <option value="complexity">complexity (bits asc)</option>
          </select>
        </label>
        <span className="bm-muted">
          {meta.computedAt
            ? "Last computed: " + meta.computedAt
            : "Not computed yet"}
          {meta.computing
            ? ` · computing… ${pct}% (${meta.done}/${meta.total})`
            : ""}
        </span>
        <button onClick={recompute} disabled={meta.computing}>
          Recompute
        </button>
      </div>

      {!meta.hasReport && !meta.computing && (
        <p className="bm-muted">No report yet. Click Recompute to build it.</p>
      )}

      {data && (
        <div className="bm-grid-wrap" ref={wrapRef}>
          <div
            className="bm-grid"
            style={{
              gridTemplateColumns: `${LABEL_W}px 1fr`,
              gridTemplateRows: `${HEADER_H}px 1fr`,
            }}
          >
            <div className="bm-corner">
              {sortedRows.length} bitmaps · diff {axisLo}–{axisHi} · max pool{" "}
              {data.pool_max}
            </div>
            <div className="bm-header-clip">
              <div className="bm-header" ref={headerRef}>
                {Array.from({ length: numCols }, (_, i) => (
                  <div className="bm-colhead" style={{ width: CELL_W }} key={i}>
                    {axisLo + i}
                  </div>
                ))}
              </div>
            </div>
            <div className="bm-rowlabels-clip" style={{ height: GRID_H }}>
              <div className="bm-rowlabels" ref={rowLabelRef}>
                {sortedRows.map((r) => (
                  <div
                    className="bm-rowhead"
                    style={{ height: CELL_H, width: LABEL_W }}
                    key={r.bitmap}
                  >
                    <span className="bm-rowbits">{r.bits.join(", ")}</span>
                    <span className="bm-rowmeta">
                      ceil {r.ceil} · use {rowUsage(r)}
                    </span>
                  </div>
                ))}
              </div>
            </div>
            <FixedSizeGrid
              className="bm-body"
              columnCount={numCols}
              columnWidth={CELL_W}
              rowCount={sortedRows.length}
              rowHeight={CELL_H}
              width={gridW}
              height={GRID_H}
              onScroll={onScroll}
            >
              {Cell}
            </FixedSizeGrid>
          </div>
        </div>
      )}
    </div>
  );
};

export { BitmapMatrixView };
