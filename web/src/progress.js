import React, { useEffect, useState } from "react";

import "./progress.scss";

// Format minutes as "Xh Ym" or "Xm" or "0m" (user-facing, no seconds)
const formatMinutes = (totalMinutes) => {
  if (totalMinutes == null || totalMinutes < 0) return "—";
  const hours = Math.floor(totalMinutes / 60);
  const minutes = Math.floor(totalMinutes % 60);
  if (hours > 0) {
    return `${hours}h ${minutes}m`;
  }
  return `${minutes}m`;
};

// Format milliseconds as "Xs" or "Xm Ys"
const formatMs = (ms) => {
  if (ms == null || ms < 0) return "—";
  const totalSec = Math.floor(ms / 1000);
  const m = Math.floor(totalSec / 60);
  const s = totalSec % 60;
  if (m > 0) {
    return `${m}m ${s}s`;
  }
  return `${s}s`;
};

const ProgressView = ({ token, apiUrl, user }) => {
  const [data, setData] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);

  useEffect(() => {
    if (!token || !apiUrl || !user || !user.id) {
      return;
    }
    const fetchProgress = async () => {
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
        const res = await fetch(apiUrl + "/statistics/" + user.id, reqParams);
        if (!res.ok) {
          setError("Could not load statistics");
          setData(null);
          return;
        }
        const json = await res.json();
        setData(json);
      } catch (e) {
        setError(e.message || "Could not load statistics");
        setData(null);
      } finally {
        setLoading(false);
      }
    };
    fetchProgress();
  }, [token, apiUrl, user]);

  if (loading) {
    return <div className="content-loading"></div>;
  }
  if (error) {
    return (
      <div className="progress-page">
        <p className="progress-error">{error}</p>
      </div>
    );
  }
  if (!data) {
    return (
      <div className="progress-page">
        <p>No statistics data.</p>
      </div>
    );
  }

  const totalWork = data.total_work_minutes ?? 0;
  const totalVideo = data.total_video_minutes ?? 0;
  const totalTime = totalWork + totalVideo;
  const workPct = totalTime > 0 ? Math.round((100 * totalWork) / totalTime) : 0;

  return (
    <div className="progress-page">
      <h1 className="progress-header">Progress</h1>

      <section className="progress-summary">
        <div className="progress-summary-item">
          <span className="progress-summary-value">
            {data.total_problems_solved ?? 0}
          </span>
          <span className="progress-summary-label">Problems solved</span>
        </div>
        <div className="progress-summary-item">
          <span className="progress-summary-value">
            {formatMinutes(data.total_work_minutes)}
          </span>
          <span className="progress-summary-label">Math time</span>
        </div>
        <div className="progress-summary-item">
          <span className="progress-summary-value">
            {formatMinutes(data.total_video_minutes)}
          </span>
          <span className="progress-summary-label">Video time</span>
        </div>
        <div className="progress-summary-item">
          <span className="progress-summary-value">{workPct}%</span>
          <span className="progress-summary-label">Time on math</span>
        </div>
      </section>

      {Array.isArray(data.stats_by_month) && data.stats_by_month.length > 0 && (
        <section className="progress-by-month">
          <h2>By month</h2>
          <table className="progress-by-month-table">
            <thead>
              <tr>
                <th>Month</th>
                <th>Problems solved</th>
                <th>Math time</th>
                <th>Video time</th>
                <th>Time on math</th>
              </tr>
            </thead>
            <tbody>
              {data.stats_by_month.map((row) => {
                const total =
                  (row.total_work_minutes ?? 0) +
                  (row.total_video_minutes ?? 0);
                const pct =
                  total > 0
                    ? Math.round((100 * (row.total_work_minutes ?? 0)) / total)
                    : 0;
                return (
                  <tr key={row.month}>
                    <td>{row.month}</td>
                    <td>{row.total_problems_solved ?? 0}</td>
                    <td>{formatMinutes(row.total_work_minutes)}</td>
                    <td>{formatMinutes(row.total_video_minutes)}</td>
                    <td>{pct}%</td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </section>
      )}

      {Array.isArray(data.hardest_problems) &&
        data.hardest_problems.length > 0 && (
          <section className="progress-hardest">
            <h2>20 hardest problems</h2>
            <p className="progress-hardest-caption">
              By average time to solve (same problem may appear multiple times)
            </p>
            <table className="progress-hardest-table">
              <thead>
                <tr>
                  <th>Problem</th>
                  <th>Avg time to solve</th>
                  <th>Avg attempts per solve</th>
                  <th>Times seen</th>
                </tr>
              </thead>
              <tbody>
                {data.hardest_problems.map((p, i) => (
                  <tr key={p.problem_id}>
                    <td className="progress-hardest-problem">
                      <span className="progress-hardest-expression">
                        {p.expression || "—"}
                      </span>
                      {p.answer != null && p.answer !== "" && (
                        <span className="progress-hardest-answer">
                          {" "}
                          = {p.answer}
                        </span>
                      )}
                    </td>
                    <td>{formatMs(p.avg_time_to_solve_ms)}</td>
                    <td>{Number(p.avg_attempts_per_solve).toFixed(1)}</td>
                    <td>{p.times_seen}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </section>
        )}
    </div>
  );
};

export { ProgressView };
