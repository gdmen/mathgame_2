// React's act() refuses to run unless the environment declares itself a test
// environment. Registered as vitest's setupFiles (web/vite.config.js).
globalThis.IS_REACT_ACT_ENVIRONMENT = true;
