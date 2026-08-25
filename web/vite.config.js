import fs from "node:fs";
import path from "node:path";

import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

// Dev-server parity with production for the static pages.
//
// In production the static marketing page IS index.html and the React shell is
// app.html (the two renames at the end of the Makefile's build-web). The dev
// server has no such swap: it serves the React shell for "/", which means the
// app's "/" route renders ToLanding, which navigates to "/", which serves the
// shell again — an infinite reload loop, and dev showing a completely
// different page from prod. Same story for the static privacy page, which
// production's front door resolves to privacy.html.
//
// Their stylesheet and fonts are already reachable in dev because
// `make landing-assets` writes them into public/, which the dev server serves
// as-is (`make dev-web` runs that target).
const staticPages = () => ({
  name: "static-pages",
  configureServer(server) {
    const pages = { "/": "landing.html", "/privacy": "privacy.html" };
    server.middlewares.use((req, res, next) => {
      const page = pages[req.url.split("?")[0]];
      if (!page) {
        next();
        return;
      }
      res.setHeader("Content-Type", "text/html; charset=utf-8");
      res.end(fs.readFileSync(path.join(import.meta.dirname, "public", page)));
    });
  },
});

export default defineConfig({
  plugins: [react(), staticPages()],
  // Every source file carries JSX under a .js name, which esbuild otherwise
  // parses as plain JavaScript.
  esbuild: {
    // .jsx repeats the default this include replaces, so a file named that way
    // is still transformed.
    include: [/\.jsx$/, /src\/.*\.js$/],
    exclude: /\/node_modules\//,
    loader: "jsx",
  },
  optimizeDeps: {
    // The React shell is the only bundled entry; without this the dev
    // dependency scan also crawls the static pages and any build output
    // sitting in the tree.
    entries: "index.html",
    esbuildOptions: { loader: { ".js": "jsx" } },
  },
  // Auth0's allowed callback URLs list this origin for local development, so a
  // silent fallback port would fail login with a callback mismatch.
  server: { port: 3000, strictPort: true },
  build: {
    // deploy/nginx/mikeymath.conf caches everything under /static/ immutably,
    // so the content-hashed output has to land there.
    assetsDir: "static",
    sourcemap: false,
    // The browser floor the bundle is compiled for. Vite's own default is
    // narrower (Chrome/Edge 107, Firefox 104, Safari 16), which would drop
    // devices the create-react-app browserslist still covered.
    target: ["es2020", "chrome87", "edge88", "firefox78", "safari14"],
  },
  test: {
    environment: "jsdom",
    globals: true,
    // src/conf.json is generated from the backend conf (gen_frontend_conf.py),
    // so it is gitignored and absent on a clean checkout. The suites that pull
    // in a module reading it get this fixture instead of whatever the machine
    // happens to have.
    alias: [
      {
        find: /^\.\/conf\.json$/,
        replacement: path.join(import.meta.dirname, "src", "conf.test.json"),
      },
    ],
  },
});
