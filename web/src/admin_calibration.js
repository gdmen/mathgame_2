import React, { useEffect, useState } from "react";
import parse from "html-react-parser";
import katex from "katex";
import "katex/dist/katex.min.css";

import { PreprocessExpression } from "./problem.js";
import "./admin_calibration.scss";

// Render a stored expression to math the same way the play page does, so the
// calibration view matches what kids actually see. Falls back to raw text if
// KaTeX can't parse it.
const renderMath = (expr) => {
  try {
    return parse(katex.renderToString(PreprocessExpression(expr)));
  } catch (e) {
    return <code>{expr}</code>;
  }
};

const fmt = (n) => (n == null ? "—" : Number(n).toFixed(2));

const nameCounts = (arr) =>
  Array.isArray(arr) && arr.length
    ? arr.map((nc) => `${nc.name} ×${nc.count}`).join(", ")
    : "—";

// generatorMix renders a bucket's generator groups as "name ×liveCount, ...".
const generatorMix = (gens) =>
  Array.isArray(gens) && gens.length
    ? gens.map((g) => `${g.generator} ×${g.live_count}`).join(", ")
    : "—";

const sampledCount = (gens) =>
  Array.isArray(gens)
    ? gens.reduce((s, g) => s + (g.problems ? g.problems.length : 0), 0)
    : 0;

// Breakdown renders the ComputeProblemDifficulty factors for one problem,
// written as the formula it is: the four factors are MULTIPLIED into raw
// (shown with ×), the concept factor is the product of its enabled multipliers
// (×), and the structure factor is built ADDITIVELY (1 + per-op + missing).
const Breakdown = ({ bd }) => {
  if (!bd) {
    return null;
  }
  // concept = product of the enabled concept multipliers.
  const conceptDetail =
    Array.isArray(bd.concepts) && bd.concepts.length
      ? " = " +
        bd.concepts
          .map((c) => `${c.name} ×${Number(c.factor).toFixed(1)}`)
          .join(" × ")
      : "";
  // structure = 1 + additive increments (per extra op, missing-number bump).
  const structAdds = [];
  if (bd.num_ops > 1) {
    structAdds.push(`${bd.num_ops - 1} extra op${bd.num_ops > 2 ? "s" : ""}`);
  }
  if (bd.has_missing) {
    structAdds.push("missing");
  }
  const structDetail = structAdds.length
    ? ` = 1 + ${structAdds.join(" + ")}`
    : "";
  return (
    <span className="calib-factors">
      raw {fmt(bd.raw)} = mag {fmt(bd.magnitude)} (max {bd.max_magnitude}) × op{" "}
      {fmt(bd.op_weight)} × concept {fmt(bd.concept)}
      {conceptDetail} × struct {fmt(bd.structure)}
      {structDetail} → scaled {fmt(bd.scaled)}
    </span>
  );
};

// Problem renders one sampled problem row (math, raw, breakdown, bits).
const Problem = ({ p }) => (
  <div className="calib-prob">
    <span className="calib-diff">{fmt(p.difficulty)}</span>
    <span className="calib-render">{renderMath(p.expression)}</span>
    <code className="calib-raw">{p.expression}</code>
    <Breakdown bd={p.breakdown} />
    <span className="calib-muted">
      [{Array.isArray(p.bits) ? p.bits.join(", ") : ""}]
    </span>
  </div>
);

const DifficultyCalibrationView = ({ token, apiUrl, user }) => {
  const [data, setData] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);

  // Access is gated by the route (non-admins get the 404 page) and enforced
  // server-side by RequireAdmin; this component just needs an authed user.
  useEffect(() => {
    if (!token || !apiUrl || !user) {
      return;
    }
    const fetchData = async () => {
      setLoading(true);
      setError(null);
      try {
        const reqParams = {
          method: "GET",
          headers: {
            Accept: "application/json",
            "Content-Type": "application/json",
            Authorization: "Bearer " + token,
          },
        };
        const res = await fetch(
          apiUrl + "/admin/difficulty-calibration",
          reqParams
        );
        if (!res.ok) {
          setError("Could not load calibration data");
          setData(null);
          return;
        }
        setData(await res.json());
      } catch (e) {
        setError(e.message || "Could not load calibration data");
        setData(null);
      } finally {
        setLoading(false);
      }
    };
    fetchData();
  }, [token, apiUrl, user]);

  if (loading) {
    return <div className="content-loading"></div>;
  }
  if (error) {
    return (
      <div className="calib-page">
        <p className="calib-error">{error}</p>
      </div>
    );
  }
  if (!data || !Array.isArray(data.buckets)) {
    return (
      <div className="calib-page">
        <p>No calibration data.</p>
      </div>
    );
  }

  return (
    <div className="calib-page">
      <h1>Difficulty calibration</h1>
      <p className="calib-hint">
        Read-only view of live problems per difficulty bucket, grouped by
        generator version: one example of each distinct problem-type bitmap that
        generator produced in the bucket. Buckets are [center−0.5, center+0.5);
        the scale is open-ended above 20 (system max ≈ 62). Each problem shows
        its ComputeProblemDifficulty factor breakdown.
      </p>

      <table className="calib-summary">
        <thead>
          <tr>
            <th>Bucket</th>
            <th>Live</th>
            <th>Disabled</th>
            <th>Examples</th>
            <th>Generator mix</th>
            <th>Dominant bits</th>
          </tr>
        </thead>
        <tbody>
          {data.buckets.map((b) => (
            <tr key={b.label}>
              <td>
                <a href={"#bucket-" + b.label}>{b.label}</a>
              </td>
              <td>{b.live_count}</td>
              <td>{b.disabled_count}</td>
              <td>{sampledCount(b.generators)}</td>
              <td>{generatorMix(b.generators)}</td>
              <td>{nameCounts(b.dominant_bits)}</td>
            </tr>
          ))}
        </tbody>
      </table>

      {data.buckets.map((b) => (
        <section
          className="calib-bucket"
          id={"bucket-" + b.label}
          key={b.label}
        >
          <h2>
            Difficulty {b.label}{" "}
            <span className="calib-muted">
              ({b.live_count} live, {b.disabled_count} disabled)
            </span>
          </h2>
          {(!b.generators || b.generators.length === 0) && (
            <p className="calib-muted">— no live problems in this bucket —</p>
          )}
          {b.generators &&
            b.generators.map((g) => (
              <div className="calib-gen" key={g.generator}>
                <h3 className="calib-gen-head">
                  {g.generator}{" "}
                  <span className="calib-muted">
                    ({g.live_count} live, {g.problems ? g.problems.length : 0}{" "}
                    distinct shapes)
                  </span>
                </h3>
                {g.problems &&
                  g.problems.map((p, i) => <Problem p={p} key={i} />)}
              </div>
            ))}
        </section>
      ))}
    </div>
  );
};

export { DifficultyCalibrationView };
