// One place builds an authenticated API request: the JSON + bearer-token
// headers every endpoint expects, and the apiUrl + path join.
const authHeaders = (token) => ({
  Accept: "application/json",
  "Content-Type": "application/json",
  Authorization: "Bearer " + token,
});

// Returns the raw Response — the caller owns the outcome. What a non-2xx means
// and how to say so differs per page, and deciding it here would flatten
// distinctions the pages depend on: a 403 that redirects, a 404 that
// provisions, a 204 with no body to read.
const apiFetch = (apiUrl, path, token, opts = {}) =>
  fetch(apiUrl + path, { method: "GET", ...opts, headers: authHeaders(token) });

export { apiFetch };
