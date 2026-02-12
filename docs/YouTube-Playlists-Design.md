# YouTube Playlists Design Doc

**Status:** Implemented (HEAD).  
**Summary:** Use YouTube playlists as the basis for user video lists. Users add one or more public YouTube playlists in Settings; the union of those playlists’ videos becomes their reward video pool for the game.

---

## 1. Goals

- Replace the previous model (users adding individual videos or a fixed deploy-time list) with **playlist-based** video sources.
- Let users manage reward videos by adding/removing **YouTube playlists** in Settings.
- Ensure the **YouTube Data API** is required and configured (no “if key empty” branching).
- Keep **one canonical video row per YouTube video** (de-duplication by `you_tube_id`).
- Support **migration** from the old schema (videos with `user_id`) to the new schema (playlists → `user_has_video`).

---

## 2. Data Model

### 2.1 New / changed tables

- **`playlists`**  
  - One row per YouTube playlist we know about.  
  - Columns: `id`, `you_tube_id` (unique), `title`, `thumbnailurl`, `etag`.  
  - `you_tube_id` is the YouTube playlist ID (e.g. `PLxxx`).

- **`playlist_video`**  
  - Many-to-many: which videos belong to which playlist.  
  - Columns: `playlist_id`, `video_id` (PK).  
  - FKs to `playlists(id)` and `videos(id)`.

- **`user_playlist`**  
  - Which playlists a user has added.  
  - Columns: `user_id`, `playlist_id` (PK).  
  - FKs to `users(id)` and `playlists(id)`.

- **`user_has_video`**  
  - Denormalized “user’s video pool”: union of all videos from all of the user’s playlists.  
  - Columns: `user_id`, `video_id` (PK).  
  - FKs to `users(id)` and `videos(id)`.  
  - Populated/refreshed when the user adds or removes a playlist (see §4).

### 2.2 Videos table (post-migration)

- **`videos`**  
  - One row per distinct YouTube video (or legacy non-YouTube video).  
  - `you_tube_id` unique, nullable.  
  - **No** `user_id` column after migration 22; ownership is expressed only via playlists → `user_has_video`.

### 2.3 Canonical video and de-duplication

- A “canonical key” for a YouTube video is derived from URL:  
  - `youtu.be/ID` → 11-char ID  
  - `v=ID` in query → 11-char ID  
  - Otherwise the full URL.  
- At most one `videos` row per canonical key; migration 22 enforces this and remaps references (events, gamestates, playlist_video, user_has_video) to the chosen “winner” row.

---

## 3. Migrations (overview)

- **15:** Create `playlists` (id, you_tube_id, thumbnailurl, etag, curated).  
- **16:** Create `playlist_video`.  
- **17:** Create `user_playlist`.  
- **18:** Create `user_has_video`.  
- **19:** Drop `playlists.curated`.  
- **20:** Add `videos.you_tube_id` (nullable, unique).  
- **21:** Drop `videos.deleted` (if present).  
- **22:** Consolidation: canonical key per video; pick winner per key; remap events (done_watching_video), gamestates, playlist_video, user_has_video; backfill `user_has_video` from `videos.user_id` **before** loser→winner remap (Step 3b); then drop loser video rows and drop `videos.user_id`.  
- **23:** Backfill `videos.you_tube_id` from URL for existing rows.  
- **24:** Add `playlists.title`.

Migration 22 is critical: it ensures one video per YouTube ID and moves ownership from `videos.user_id` into `user_has_video` (with Step 3b so user-owned videos are in `user_has_video` before the remap).

---

## 4. Backend Behavior

### 4.1 Config

- **`conf.json`** must set all required fields. Validation uses reflection over the config struct: every **string** field must be non-empty (after trim).  
- **`youtube_api_key`** is required; there is no “if key empty” path.  
- Validation runs at startup (apiserver, auth0, llm_generator, verify_migrations) and on config read where applicable.

### 4.2 Playlist API

- **GET /api/v1/playlists**  
  - Returns the authenticated user’s playlists (join `user_playlist` + `playlists`).  
  - Each item includes id, you_tube_id, title, thumbnailurl, etag.

- **POST /api/v1/playlists**  
  - Body: one of `playlist_id`, `youtube_playlist_id`, or `playlist_url`.  
  - If URL or YouTube ID: call **syncPlaylistFromYouTube** (fetch metadata + items from YouTube Data API, upsert `playlists` and `playlist_video`, create missing `videos`).  
  - Insert into `user_playlist`, then **refreshUserHasVideo** for that user.  
  - Response: `{ "id": playlistId }`.

- **DELETE /api/v1/playlists/:playlist_id**  
  - Delete from `user_playlist` for current user and that playlist.  
  - Then **refreshUserHasVideo** for that user.

### 4.3 refreshUserHasVideo(userId)

- Delete all `user_has_video` rows for that user.  
- Insert the union of (user’s playlists × playlist_video):  
  `INSERT INTO user_has_video (user_id, video_id) SELECT DISTINCT up.user_id, pv.video_id FROM user_playlist up INNER JOIN playlist_video pv ON up.playlist_id = pv.playlist_id WHERE up.user_id = ?`

So the “source of truth” for “which videos can this user see” is the union of their playlists; `user_has_video` is a cached copy refreshed on add/remove playlist.

### 4.4 Video list and play eligibility

- **GET /api/v1/videos**  
  - Returns videos for the current user: join `videos` with `user_has_video` for that user (same as before; now fed by playlists).

- **Pageload**  
  - `num_videos_enabled`: count of rows in `user_has_video` for that user with `videos.disabled = 0`.

- **Play**  
  - Requires at least 3 enabled videos (count from `user_has_video` + non-disabled).  
  - Error message: “Add at least 3 videos via playlists in Settings to play.”

### 4.5 YouTube sync (syncPlaylistFromYouTube)

- **Metadata:** `GET youtube/v3/playlists?part=snippet&id=...` → title, thumbnail, etag.  
- **Items:** Paginate `youtube/v3/playlistItems?playlistId=...`; for each item get videoId, title, thumbnail.  
- **DB:**  
  - Upsert `playlists` (by you_tube_id): insert or update title/thumbnail/etag.  
  - Clear `playlist_video` for this playlist, then for each video: ensure a `videos` row (by you_tube_id), then insert into `playlist_video`.  
- New videos are created with title, url (`https://www.youtube.com/watch?v=ID`), thumbnailurl, you_tube_id.

---

## 5. Frontend (Settings)

- **Your playlists**  
  - List of the user’s playlists (from GET /playlists), each showing **title** (or you_tube_id or “Playlist &lt;id&gt;”).  
  - Each playlist **title is a link** to `https://www.youtube.com/playlist?list={you_tube_id}` (opens in new tab).  
  - Input: “YouTube playlist URL or ID”; on submit, POST /playlists with `playlist_url` or `youtube_playlist_id` as appropriate.  
  - Remove: DELETE /playlists/:id; list and “videos from your playlists” refresh.

- **Videos from your playlists**  
  - Read-only list of videos (union of playlists), with note that adding/removing is done via YouTube or by removing a playlist above.

---

## 6. Migration Verification and Cleanup

- **verify_migrations**  
  - Compares “before” and “after” DBs (e.g. pre– and post–migration 22).  
  - Checks: gamestate.video_id in videos; done_watching_video event values in videos; user_has_video.video_id in videos; playlist_video.video_id in videos; no duplicate you_tube_id in videos; user-video remap (every (user_id, video_id) from before.videos or user_has_video is represented in after user_has_video).  
  - On failure, prints **detailed** rows (e.g. gamestate id, user_id, video_id; event id, value; user_id, before_video_id, canonical_key, after_winner_video_id) so failures are debuggable.

- **make clean**  
  - Runs **clean_test_dbs** (optional step, non-fatal): drops MySQL databases matching `{mysql_database}_%` (e.g. mathgame_test_1, mathgame_test_2) from `test_conf.json` so aborted test runs don’t leave lingering test DBs.

---

## 7. Testing

- **API tests** use a **per-test database**: `setupTestAPI(t, c)` creates a unique DB (e.g. mathgame_test_1, mathgame_test_2), runs migrations, and returns `(api, router, cleanup)`.  
- Tests run **in parallel**; each has its own DB and no shared global API.  
- **Playlist tests:** list playlists (empty and with data), add playlist by playlist_id (videos appear in GET /videos), remove playlist (user_has_video and GET /videos updated).  
- **Test auth:** For POST requests with a JSON body, `test_auth0_id` is read from the **query string** so the test user is correctly resolved when adding playlists.  
- **insertVideosAndUserHasVideo** uses a per-call unique prefix for `you_tube_id` so subtests (e.g. TestPlay_RequiresThreeVideos) don’t hit the unique constraint.

---

## 8. File / Component Summary

| Area | Files / components |
|------|--------------------|
| Config | `server/common/config.go` (Validate via reflection; ReadConfig) |
| Migrations | `server/api/migrations/15–24.sql` |
| Playlist API | `server/api/custom_handlers.go` (customListPlaylists, customAddPlaylist, customRemovePlaylist, refreshUserHasVideo) |
| YouTube | `server/api/youtube.go` (fetchPlaylistMetadata, fetchPlaylistItems, syncPlaylistFromYouTube) |
| Video list / play | `customListVideo`, `countEnabledVideosForUser`, `customGetPlayData`, `customGetPageLoadData` (user_has_video) |
| Settings UI | `web/src/settings.js` (PlaylistsSettingsView, playlist link to YouTube), `web/src/settings.scss` |
| Verify | `cmd/verify_migrations/main.go` (detailed failure output) |
| Clean | `cmd/clean_test_dbs/main.go`, `Makefile` (clean target) |
| Tests | `server/api/*_test.go` (setupTestAPI, playlist tests, parallel per-test DBs) |

---

## 9. Out of Scope / Removed

- **Deploy-time video list** (e.g. deploy/videos.csv, gen_video_sql.py) removed; videos come from YouTube playlists only (or existing DB state).  
- **Optional YouTube API key** removed; key is required and validated at startup.  
- **conf.debug** removed; unused.  
- **Direct “add one video” as primary flow** replaced by “add playlist”; legacy video rows remain supported via migration and user_has_video.

This document reflects the implementation as of the HEAD commit and the follow-on changes (config validation, migration Step 3b, verify_migrations details, test parallelism, clean_test_dbs, settings playlist link, and test auth fix).
