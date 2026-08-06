import { apiFetch } from "./api.js";

// Every authenticated request in the app is built here, and nothing else
// asserts on the headers — a dropped Authorization header would otherwise
// reach production as a blanket 401 with no failing test.
const lastCall = () =>
  global.fetch.mock.calls[global.fetch.mock.calls.length - 1];

beforeEach(() => {
  global.fetch = jest.fn(() => Promise.resolve({ ok: true }));
});

afterEach(() => {
  delete global.fetch;
});

test("sends the bearer token and the JSON headers", async () => {
  await apiFetch("/api/v1", "/play/1", "tok");
  const [, opts] = lastCall();
  expect(opts.headers).toEqual({
    Accept: "application/json",
    "Content-Type": "application/json",
    Authorization: "Bearer tok",
  });
});

test("joins apiUrl and path, and defaults to GET", async () => {
  await apiFetch("/api/v1", "/statistics/7", "tok");
  const [url, opts] = lastCall();
  expect(url).toBe("/api/v1/statistics/7");
  expect(opts.method).toBe("GET");
});

test("opts carry the method and body through", async () => {
  const body = JSON.stringify({ pin: "1234" });
  await apiFetch("/api/v1", "/users/abc", "tok", { method: "DELETE", body });
  const [url, opts] = lastCall();
  expect(url).toBe("/api/v1/users/abc");
  expect(opts.method).toBe("DELETE");
  expect(opts.body).toBe(body);
});

// The auth headers are applied after opts are spread, so a caller cannot
// clobber Authorization by passing headers of their own.
test("opts cannot displace the auth headers", async () => {
  await apiFetch("/api/v1", "/playlists", "tok", {
    headers: { Authorization: "Bearer nope" },
  });
  const [, opts] = lastCall();
  expect(opts.headers.Authorization).toBe("Bearer tok");
});

test("returns the response untouched, so callers own the outcome", async () => {
  const res = { ok: false, status: 403 };
  global.fetch = jest.fn(() => Promise.resolve(res));
  await expect(apiFetch("/api/v1", "/play/1", "tok")).resolves.toBe(res);
});
