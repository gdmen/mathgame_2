# YouTube playlist sync

How a YouTube playlist becomes rows in `playlists`, `videos`, and `playlist_video`. This area owns
`server/api/youtube.go` — the sync layer that talks to the YouTube Data API and reconciles its
response into the DB. **Change this doc in the same PR as any behavior change here**;
`TestDocsSyncVideos` (`server/api/docs_sync_test.go`) pins the anchor block below to code and fails
CI on drift. The full data model (tables + join tables) lives in `docs/schema.md`.

<!-- BEGIN DOC-SYNC ANCHORS (parsed by server/api/docs_sync_test.go) -->
```
youtube_api_host: https://www.googleapis.com/youtube/v3
playlist_items_page_size: 50
video_watch_url_prefix: https://www.youtube.com/watch?v=
youtube_api_key_required: true
```
<!-- END DOC-SYNC ANCHORS -->

## The model

`youtube.go` is a thin sync layer with no HTTP handler of its own. Its single entry point is
`syncPlaylistFromYouTube`, called from `customAddPlaylist` (`custom_handlers.go`) when a user adds a
playlist by URL or YouTube ID. Adding by an existing internal `playlist_id` skips sync entirely
(`customAddPlaylist` — the `body.PlaylistID != nil` branch).

| Entity | Table | Key | Written by |
|---|---|---|---|
| Playlist | `playlists` | `you_tube_id` (unique) | `syncPlaylistFromYouTube` via `playlistManager.Create`/`Update` |
| Video | `videos` | `you_tube_id` (unique) | `syncPlaylistFromYouTube` (raw `INSERT`, dedup by `you_tube_id`) |
| Membership | `playlist_video` | `(playlist_id, video_id)` | `syncPlaylistFromYouTube` (cleared then rebuilt per sync) |

`syncPlaylistFromYouTube` maintains only the canonical playlist/video/membership rows; it never
touches `user_playlist` or `user_has_video`. The caller inserts `user_playlist` and then calls
`refreshUserHasVideo` (`custom_handlers.go`) to rebuild the user's pool from the union of their
playlists. Ownership lives one layer up.

## The YouTube API calls

Two endpoints on the YouTube Data API v3, both authenticated with `a.YouTubeAPIKey` — the
`youtube_api_key` config field, **required** because it is not in `optionalConfigFields`, so
`Config.Validate` (`server/common/config.go`) rejects an empty value.

| Call | Endpoint | Function |
|---|---|---|
| Playlist metadata | `playlists?part=snippet&id=...` | `fetchPlaylistMetadata` |
| Playlist items | `playlistItems?part=snippet&playlistId=...&maxResults=50` | `fetchPlaylistItems` |

- `fetchPlaylistMetadata` returns the playlist title, thumbnail, and etag. An empty `Items` array
  is a `"playlist not found"` error; a non-200 surfaces the response body.
- `fetchPlaylistItems` paginates on `nextPageToken` until it is empty, 50 items per page, yielding
  `{VideoID, Title, ThumbnailURL}` per item. A non-200 on any page aborts the whole fetch.

Both prefer the `medium` thumbnail and fall back to `default` — see the thumbnail gotcha below.

## The sync flow

`syncPlaylistFromYouTube(playlistID)` returns the internal `playlists.id`:

```
[1] metadata   fetchPlaylistMetadata -> title, thumbURL, etag
[2] upsert     SELECT playlists.id WHERE you_tube_id = playlistID
                 not found -> playlistManager.Create
                 found     -> playlistManager.Update (refresh title/thumb/etag)
[3] items      fetchPlaylistItems (paginated)
[4] reset      DELETE FROM playlist_video WHERE playlist_id = <id>
[5] per item   SELECT videos.id WHERE you_tube_id = VideoID
                 not found -> INSERT videos (title, url, thumbnailurl, you_tube_id)
                 INSERT playlist_video (playlist_id, video_id)
```

- Step 4 makes membership **authoritative to the latest YouTube response**: a video removed from
  the playlist on YouTube disappears from `playlist_video` on the next sync. The `videos` row itself
  is never deleted here — it may still belong to other playlists, events, or gamestates.
- Step 5 dedups videos by `you_tube_id`: a video already known from another playlist is reused, not
  re-inserted. New video URLs are synthesized as `<video_watch_url_prefix><VideoID>`.

## Error handling and partial-failure behavior

| Failure | Effect |
|---|---|
| metadata fetch / decode / not-found | whole sync aborts before any write |
| playlist `Create`/`Update` fails | whole sync aborts |
| items fetch fails (any page) | whole sync aborts AFTER the playlist upsert has committed |
| single video `INSERT` fails | logged, that video skipped, sync proceeds (`syncPlaylistFromYouTube` — the per-item `continue`) |
| `playlist_video` insert fails | logged unless the error contains `Duplicate entry`, which is swallowed |

Per-video failures are non-fatal and best-effort, but a failed items fetch leaves the playlist row
upserted with `playlist_video` already cleared (step 4 ran). A transient YouTube error can therefore
momentarily empty a playlist's membership until the next successful sync.

## Invariants

- One `playlists` row per `you_tube_id`; one `videos` row per `you_tube_id` — enforced by the unique
  keys plus the SELECT-before-INSERT dedup.
- After a successful sync, `playlist_video` for that playlist reflects exactly the videos in
  YouTube's current response (clear-and-rebuild).
- `syncPlaylistFromYouTube` never writes `user_playlist` or `user_has_video`; the caller owns
  user-pool reconciliation.

## Gotchas

- **Medium-thumbnail tag bug (#276).** In both response structs the medium thumbnail's inner field
  is tagged `json:"medium"` (`YouTubePlaylistResponse`, `YouTubePlaylistItemsResponse`), but the
  YouTube Data API returns the URL under the key `url` (as the sibling `Default.URL` is correctly
  tagged). `Thumbnails.Medium.URL` therefore always decodes empty and every playlist/video silently
  falls back to the smaller `default` thumbnail. The fallback masks the bug — it never errors.
  Surfaced, not yet fixed.
- **No transaction.** The flow is a sequence of independent `Exec`/`Query` calls, not one
  transaction. An abort mid-flow leaves partial state (see the partial-failure table). Re-running
  the sync is the recovery path and is idempotent for the playlist/video/membership rows.
- **The playlist row's thumbnail comes only from the metadata call.** Per-item thumbnails from
  `fetchPlaylistItems` are stored per video; the playlist's own thumbnail never derives from its
  items.
- **No cap on total items.** Pagination continues until `nextPageToken` is empty, so a very large
  playlist makes many sequential blocking HTTP calls inside the request that triggered the add.

## Related files

- `server/api/youtube.go` — this area (`fetchPlaylistMetadata`, `fetchPlaylistItems`,
  `syncPlaylistFromYouTube`).
- `server/api/custom_handlers.go` — `customAddPlaylist` (the only caller), `customRemovePlaylist`,
  and `refreshUserHasVideo` (the user-pool side).
- `server/api/playlist_model.generated.go` — `Playlist` model and `playlistManager`
  (Create/Update/Get); generated from `models.json`.
- `server/common/config.go` — `YouTubeAPIKey` (`youtube_api_key`), required via `Config.Validate`.
- `docs/schema.md` — the `playlists`/`videos` tables and the `playlist_video`/`user_playlist`/
  `user_has_video` join tables.
- `docs/settings.md` — the user-facing playlist add/remove UI.

## Extension checklist (changing the sync)

1. New YouTube API field needed → add it to the `YouTubePlaylist*Response` struct with the correct
   JSON tag (mind the `url` key — see the thumbnail gotcha).
2. New DB column on `videos`/`playlists` → migration + regenerate the model from `models.json`
   (`make build-api`), then thread it through the `INSERT`/`Update` in `syncPlaylistFromYouTube`.
3. Changing membership semantics (e.g. soft-delete instead of clear-and-rebuild) → update step 4 and
   the invariants above.
4. If the YouTube host, page size, watch-URL prefix, or the key-required rule changes, update the
   DOC-SYNC anchor block (the test fails CI otherwise).
