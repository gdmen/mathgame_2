import React from "react";
import ReactDOM from "react-dom";
import { act } from "react-dom/test-utils";

import { PlaylistsSettingsView, UNDO_WINDOW_MS } from "./settings.js";

// Removal has no confirm step; the undo offer is the whole safety net, so its
// mechanics are pinned here: the bar takes the removed row's own slot, undo
// puts the row back in that slot, the offer expires on its own clock, and —
// the part with a regression behind it — each removal gets its own bar and
// its own clock, so a second removal can neither evict the first offer nor
// shorten its window.

const PLAYLISTS = [
  { id: 1, title: "Alpha", video_count: 5, playable_count: 5 },
  { id: 2, title: "Beta", video_count: 4, playable_count: 4 },
  { id: 3, title: "Gamma", video_count: 3, playable_count: 3 },
];

// A stateful stand-in for the three playlist endpoints the component talks
// to. DELETE and POST mutate it the way the server would, and GET reports id
// order — the order customListPlaylists guarantees, and the order rows and
// undo offers are slotted by.
let serverPlaylists;
const installFetch = ({ failRestore = false } = {}) => {
  global.fetch = jest.fn((url, opts = {}) => {
    const method = opts.method || "GET";
    if (method === "DELETE") {
      const id = Number(url.split("/").pop());
      serverPlaylists = serverPlaylists.filter((p) => p.id !== id);
      return Promise.resolve({ ok: true });
    }
    if (method === "POST") {
      if (failRestore) return Promise.resolve({ ok: false });
      const { playlist_id } = JSON.parse(opts.body);
      serverPlaylists = PLAYLISTS.filter(
        (p) =>
          p.id === playlist_id || serverPlaylists.some((s) => s.id === p.id)
      );
      return Promise.resolve({
        ok: true,
        json: () => Promise.resolve({ id: playlist_id }),
      });
    }
    return Promise.resolve({
      ok: true,
      json: () =>
        Promise.resolve({
          playlists: serverPlaylists.slice(),
          playable_total: serverPlaylists.reduce(
            (sum, p) => sum + p.playable_count,
            0
          ),
        }),
    });
  });
};

// The list as a parent reads it, rows and offers interleaved — the shape the
// id-ordered slotting exists to get right.
const listShape = (container) =>
  [...container.querySelectorAll("ul#playlist-list > li")].map((li) =>
    li.classList.contains("playlist-undo")
      ? "undo:" + li.querySelector(".playlist-undo-text").textContent
      : li.querySelector(".playlist-title").textContent
  );

const click = (button) =>
  button.dispatchEvent(new MouseEvent("click", { bubbles: true }));

const removePlaylist = (container, title) =>
  act(async () => {
    const item = [...container.querySelectorAll("li.playlist-item")].find(
      (li) => li.querySelector(".playlist-title").textContent === title
    );
    click(item.querySelector("button.playlist-remove"));
  });

const undoRemoval = (container, title) =>
  act(async () => {
    const bar = [...container.querySelectorAll("li.playlist-undo")].find((li) =>
      li.querySelector(".playlist-undo-text").textContent.includes(title)
    );
    click(bar.querySelector("button.playlist-undo-action"));
  });

describe("playlist removal undo", () => {
  let container;

  beforeEach(async () => {
    jest.useFakeTimers();
    serverPlaylists = PLAYLISTS.slice();
    installFetch();
    container = document.createElement("div");
    document.body.appendChild(container);
    await act(async () => {
      ReactDOM.render(
        <PlaylistsSettingsView token="t" apiUrl="/api/v1" user={{ id: 1 }} />,
        container
      );
    });
  });

  afterEach(() => {
    act(() => {
      ReactDOM.unmountComponentAtNode(container);
    });
    container.remove();
    jest.useRealTimers();
    delete global.fetch;
  });

  it("puts the undo offer in the removed row's own slot", async () => {
    await removePlaylist(container, "Beta");
    expect(listShape(container)).toEqual([
      "Alpha",
      "undo:Removed “Beta”",
      "Gamma",
    ]);
  });

  it("undo puts the row back where it was", async () => {
    await removePlaylist(container, "Beta");
    await undoRemoval(container, "Beta");
    expect(listShape(container)).toEqual(["Alpha", "Beta", "Gamma"]);
    const restore = global.fetch.mock.calls.find(
      ([, opts]) => opts && opts.method === "POST"
    );
    // By id: re-attach the playlist the parent had, not a fresh URL sync.
    expect(JSON.parse(restore[1].body)).toEqual({ playlist_id: 2 });
  });

  it("the offer expires on its own", async () => {
    await removePlaylist(container, "Beta");
    act(() => {
      jest.advanceTimersByTime(UNDO_WINDOW_MS + 1);
    });
    expect(listShape(container)).toEqual(["Alpha", "Gamma"]);
  });

  it("a second removal neither evicts the first offer nor shortens its window", async () => {
    await removePlaylist(container, "Beta");
    act(() => {
      jest.advanceTimersByTime(UNDO_WINDOW_MS / 2);
    });
    await removePlaylist(container, "Gamma");
    expect(listShape(container)).toEqual([
      "Alpha",
      "undo:Removed “Beta”",
      "undo:Removed “Gamma”",
    ]);
    // Past Beta's expiry but inside Gamma's window.
    act(() => {
      jest.advanceTimersByTime(UNDO_WINDOW_MS / 2 + 1);
    });
    expect(listShape(container)).toEqual(["Alpha", "undo:Removed “Gamma”"]);
    act(() => {
      jest.advanceTimersByTime(UNDO_WINDOW_MS / 2);
    });
    expect(listShape(container)).toEqual(["Alpha"]);
  });

  it("offers hold their slots regardless of removal order", async () => {
    // Removing a later row first, then an earlier one, is what made captured
    // positions go stale: the second removal shifted the list under the
    // first offer's saved index.
    await removePlaylist(container, "Gamma");
    await removePlaylist(container, "Beta");
    expect(listShape(container)).toEqual([
      "Alpha",
      "undo:Removed “Beta”",
      "undo:Removed “Gamma”",
    ]);
  });

  it("each offer undoes independently", async () => {
    await removePlaylist(container, "Beta");
    await removePlaylist(container, "Gamma");
    await undoRemoval(container, "Gamma");
    expect(listShape(container)).toEqual([
      "Alpha",
      "undo:Removed “Beta”",
      "Gamma",
    ]);
  });

  // The failure is usually transient, so the offer has to survive it — with a
  // fresh window, since the clock was stopped for the request. Dropping it
  // would leave the parent with the playlist gone and nothing left to press.
  it("a failed restore keeps the offer and re-arms its clock", async () => {
    await removePlaylist(container, "Beta");
    act(() => {
      jest.advanceTimersByTime(UNDO_WINDOW_MS - 1000);
    });
    installFetch({ failRestore: true });
    await undoRemoval(container, "Beta");
    expect(listShape(container)).toEqual([
      "Alpha",
      "undo:Removed “Beta”",
      "Gamma",
    ]);
    expect(container.querySelector(".playlist-error").textContent).toMatch(
      /Could not bring that playlist back/
    );
    // Past the original deadline, inside the new one.
    act(() => {
      jest.advanceTimersByTime(2000);
    });
    expect(listShape(container)).toContain("undo:Removed “Beta”");
    act(() => {
      jest.advanceTimersByTime(UNDO_WINDOW_MS);
    });
    expect(listShape(container)).toEqual(["Alpha", "Gamma"]);
  });

  it("a retried restore succeeds", async () => {
    await removePlaylist(container, "Beta");
    installFetch({ failRestore: true });
    await undoRemoval(container, "Beta");
    installFetch();
    await undoRemoval(container, "Beta");
    expect(listShape(container)).toEqual(["Alpha", "Beta", "Gamma"]);
  });
});
