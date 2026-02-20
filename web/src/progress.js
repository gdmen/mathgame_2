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
        const res = await fetch(apiUrl + "/progress/" + user.id, reqParams);
        if (!res.ok) {
          setError("Could not load progress");
          setData(null);
          return;
        }
        const json = await res.json();
        setData(json);
      } catch (e) {
        setError(e.message || "Could not load progress");
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
        <p>No progress data.</p>
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
    </div>
  );
};

export { ProgressView };
