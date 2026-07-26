// Dev-server parity with production for the "/" route (#329).
//
// In production the static marketing page IS index.html and the React shell is
// app.html (the two renames at the end of the Makefile's build-web). The CRA
// dev server has no such swap: it serves the React shell for "/", which means
// the app's "/" route renders ToLanding, which navigates to "/", which serves
// the shell again — an infinite reload loop, and dev showing a completely
// different page from prod.
//
// Serving the real landing here fixes both. Its stylesheet and fonts are
// already reachable in dev because `make landing-assets` writes them into
// public/, which the dev server serves as-is (`make dev-web` runs that target).
//
// CRA loads this file automatically when it exists; nothing imports it.
const path = require("path");

module.exports = function (app) {
  app.get("/", function (req, res) {
    res.sendFile(path.join(__dirname, "..", "public", "landing.html"));
  });
};
