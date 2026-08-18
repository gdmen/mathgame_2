import React from "react";
import ReactDOM from "react-dom";
import { act } from "react-dom/test-utils";

import { PlaylistsSettingsView, RECOMMENDED_PLAYLISTS } from "./settings.js";

// The recommendations are hand-maintained links to other people's playlists,
// so the things that can quietly break them are pinned here: an entry whose
// id the server would reject, a row's Add posting something other than that
// id, and a suggestion lingering after the parent already has it.

// A stateful stand-in for GET and POST /playlists: adding by youtube_playlist_id
// puts a matching row in the list, the way the server does after a sync.
let serverPlaylists;
const installFetch = ({ failAdd = false } = {}) => {
  global.fetch = jest.fn((url, opts = {}) => {
    if ((opts.method || "GET") === "POST") {
      if (failAdd) {
        return Promise.resolve({
          ok: false,
          json: () => Promise.resolve({ message: "Playlist must be public." }),
        });
      }
      const { youtube_playlist_id, playlist_url } = JSON.parse(opts.body);
      const ytId =
        youtube_playlist_id || new URL(playlist_url).searchParams.get("list");
      serverPlaylists = [
        ...serverPlaylists,
        {
          id: serverPlaylists.length + 1,
          you_tube_id: ytId,
          title: "Synced " + ytId,
          video_count: 3,
          playable_count: 3,
        },
      ];
      return Promise.resolve({ ok: true, json: () => Promise.resolve({}) });
    }
    return Promise.resolve({
      ok: true,
      json: () =>
        Promise.resolve({
          playlists: serverPlaylists.slice(),
          playable_total: serverPlaylists.length * 3,
        }),
    });
  });
};

const click = (button) =>
  button.dispatchEvent(new MouseEvent("click", { bubbles: true }));

const typeUrl = (container, value) =>
  act(async () => {
    const input = container.querySelector("#playlist-inputs input");
    Object.getOwnPropertyDescriptor(
      window.HTMLInputElement.prototype,
      "value",
    ).set.call(input, value);
    input.dispatchEvent(new Event("input", { bubbles: true }));
  });

const suggestionTitles = (container) =>
  [...container.querySelectorAll(".recommended-title")].map(
    (a) => a.textContent,
  );

const render = (container) =>
  act(async () => {
    ReactDOM.render(
      <PlaylistsSettingsView token="t" apiUrl="/api/v1" user={{ id: 1 }} />,
      container,
    );
  });

describe("recommended playlists", () => {
  let container;

  beforeEach(async () => {
    serverPlaylists = [];
    installFetch();
    container = document.createElement("div");
    document.body.appendChild(container);
    await render(container);
  });

  afterEach(() => {
    act(() => {
      ReactDOM.unmountComponentAtNode(container);
    });
    container.remove();
    delete global.fetch;
  });

  it("every entry carries what a row needs, keyed by a plausible playlist id", () => {
    expect(RECOMMENDED_PLAYLISTS.length).toBeGreaterThan(0);
    RECOMMENDED_PLAYLISTS.forEach((rec) => {
      // YouTube playlist ids start with PL; the server takes the id verbatim.
      expect(rec.you_tube_id).toMatch(/^PL[\w-]+$/);
      expect(rec.title).toBeTruthy();
      expect(rec.channel).toBeTruthy();
      expect(rec.video_count).toBeGreaterThan(0);
      expect(rec.thumbnailurl).toMatch(/^https:\/\/i\.ytimg\.com\//);
    });
  });

  it("renders one row per entry, titled and linked out to YouTube", () => {
    const links = [...container.querySelectorAll(".recommended-title")];
    expect(links.map((a) => a.textContent)).toEqual(
      RECOMMENDED_PLAYLISTS.map((p) => p.title),
    );
    expect(links.map((a) => a.getAttribute("href"))).toEqual(
      RECOMMENDED_PLAYLISTS.map(
        (p) => "https://www.youtube.com/playlist?list=" + p.you_tube_id,
      ),
    );
  });

  it("Add posts the entry's id and the row moves into the parent's list", async () => {
    const first = RECOMMENDED_PLAYLISTS[0];
    await act(async () => {
      click(container.querySelector("button.recommended-add"));
    });
    const post = global.fetch.mock.calls.find(
      ([, opts]) => opts && opts.method === "POST",
    );
    expect(JSON.parse(post[1].body)).toEqual({
      youtube_playlist_id: first.you_tube_id,
    });
    expect(
      [...container.querySelectorAll(".playlist-title")].map(
        (t) => t.textContent,
      ),
    ).toEqual(["Synced " + first.you_tube_id]);
    expect(suggestionTitles(container)).toEqual(
      RECOMMENDED_PLAYLISTS.slice(1).map((p) => p.title),
    );
  });

  it("does not suggest a playlist the parent already has", async () => {
    const second = RECOMMENDED_PLAYLISTS[1];
    serverPlaylists = [
      {
        id: 7,
        you_tube_id: second.you_tube_id,
        title: "Mine",
        video_count: 3,
        playable_count: 3,
      },
    ];
    await render(container);
    expect(suggestionTitles(container)).toEqual(
      RECOMMENDED_PLAYLISTS.filter((p) => p !== second).map((p) => p.title),
    );
  });

  it("a failed Add shows the server's reason and keeps the row", async () => {
    installFetch({ failAdd: true });
    await act(async () => {
      click(container.querySelector("button.recommended-add"));
    });
    expect(container.querySelector(".playlist-error").textContent).toBe(
      "Playlist must be public.",
    );
    expect(suggestionTitles(container)).toEqual(
      RECOMMENDED_PLAYLISTS.map((p) => p.title),
    );
    expect(container.querySelector("button.recommended-add").disabled).toBe(
      false,
    );
  });

  it("a recommendation Add clears the error a failed URL add left behind", async () => {
    installFetch({ failAdd: true });
    await typeUrl(container, "https://www.youtube.com/playlist?list=PLprivate");
    await act(async () => {
      click(container.querySelector("#playlist-inputs button"));
    });
    expect(container.querySelector(".playlist-error")).not.toBeNull();

    installFetch();
    await act(async () => {
      click(container.querySelector("button.recommended-add"));
    });
    expect(container.querySelector(".playlist-error")).toBeNull();
  });
});
