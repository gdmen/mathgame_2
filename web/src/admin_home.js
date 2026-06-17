import React from "react";

import "./admin_home.scss";

// The admin tools linked from the admin home. Each is its own admin-gated route.
const tools = [
  {
    path: "/admin/difficulty-calibration",
    label: "Difficulty calibration",
    desc: "Sample the live pool per difficulty bucket to calibrate ComputeProblemDifficulty.",
  },
  {
    path: "/admin/style-guide",
    label: "Style guide",
    desc: "Design tokens and component patterns.",
  },
];

const AdminHomeView = () => (
  <div className="admin-home">
    <h1>Admin</h1>
    <ul className="admin-tools">
      {tools.map((t) => (
        <li key={t.path}>
          <a href={t.path}>
            <span className="admin-tool-label">{t.label}</span>
            <span className="admin-tool-desc">{t.desc}</span>
          </a>
        </li>
      ))}
    </ul>
  </div>
);

export { AdminHomeView };
