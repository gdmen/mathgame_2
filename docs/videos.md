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

**Videos are playlist-synced only.** A user's video pool is the union of the videos in the
playlists they subscribe to, and nothing else: there is no supported notion of an individually
added video. The only way a *user* changes their own pool is by changing which playlists they
have — `POST /playlists` and `DELETE /playlists/:playlist_id` — after which
`refreshUserHasVideo` rebuilds `user_has_video` from `user_playlist`, wiping first, so any row the
union does not back disappears. `TestUserHasVideo_IsThePlaylistUnionAfterEveryMutation`
(`server/api/user_has_video_invariant_test.go`) pins the equality across a sequence of adds and
removes over overlapping playlists.

Two things reach a pool from outside that path, and neither goes through a playlist the user
touched. The server disables a video nobody could play (see the invariants below), and a
membership resync run for one subscriber rewrites `playlist_video` for every subscriber
(see [Staleness across users](#staleness-across-users)).

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

Both prefer the `medium` thumbnail and fall back to `default` when it is absent. Each thumbnail
size is an object keyed by size (`default`, `medium`) whose URL lives under the inner key `url`.

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
- **`user_has_video` equals the union of that user's playlists after every playlist mutation they
  make.** It is derived state, rebuilt rather than amended, so it is never the record of a
  decision. The qualifier is load-bearing: the rebuild is per-user, so it settles the acting
  user's pool and nobody else's (see [Staleness across users](#staleness-across-users)).
- **A pool that shrinks takes the selected reward with it.** `/play` resolves `gamestate.video_id`
  by id alone, with no `user_has_video` join, so both callers of `refreshUserHasVideo` follow it
  with `selectVideoIfUnavailable` (`custom_handlers.go`): if the current reward is no longer
  drawable from the pool, the gamestate is repointed at one that is. Removing a playlist is the
  obvious shrink; adding one is the less obvious one, because the sync clears and rebuilds
  `playlist_video` (step 4), so re-adding a playlist can drop the reward. The check is pool
  membership, not equality with a single id, because one mutation can drop many videos. An emptied
  pool leaves `nullVideoId`, which is what a user with no videos already carries.
- **The `/videos` routes are read-only.** Only `GET /videos`, `GET /videos/:id` and
  `GET /playlists/:playlist_id/videos` are registered; the generated `createVideo`, `updateVideo`
  and `deleteVideo` handlers exist but are unrouted, like `listUser`. The `TestVideoBasic`
  "Create: not exposed", "Update: not exposed" and "Delete: not exposed" steps pin that. Two
  separate reasons converge on it:
  - A `videos` row is **shared**. It is keyed by nothing but its own id and is reachable by every
    user who has that video, so a per-video write endpoint would let any caller rewrite the title,
    URL or `disabled` flag for all of them.
  - A user's pool is **derived**. Creating or removing one entry in it authors state the next
    playlist mutation rebuilds from scratch, so the write would be silently undone rather than
    honored. Playlists are the only control over a pool.
- **The only writes to `videos` are server-initiated.** This sync; the server disabling a video it
  couldn't play (`videoManager.Update` on `ERROR_PLAYING_VIDEO`, see [events.md](events.md)); and
  `cmd/check_disabled_videos` re-enabling one. Note the disable is **global, not per-user**: it
  sets `videos.disabled` on the shared row, and `selectVideo` and the playable counts filter on
  `v.disabled = 0`, so one child's unplayable video leaves every pool that contains it. There is no
  per-user video-level control at all — the granularity a user has is the playlist.

## Staleness across users

The pool equality holds per user at the moment that user mutates a playlist. It is not a
continuously maintained invariant across accounts, because two of the writes above are global while
the rebuild is not:

- `syncPlaylistFromYouTube` clears and rebuilds `playlist_video` for the whole playlist (step 4),
  but `customAddPlaylist` then calls `refreshUserHasVideo` for **the acting user only**. A second
  account already subscribed to that playlist keeps its old `user_has_video` rows — including rows
  for videos the resync just removed from the playlist — until it next adds or removes a playlist
  of its own.
- `ERROR_PLAYING_VIDEO` flips `videos.disabled` on the shared row, which every pool reads through
  the `v.disabled = 0` filter rather than through `user_has_video`, so that one takes effect for
  everyone immediately.

The consequence is real but narrow: until that user's next playlist mutation, a video dropped from
the playlist upstream can still be drawn as their reward, because `selectVideo` filters on
`v.disabled = 0` and not on current playlist membership. Nothing serves a video the user never had
access to — the stale rows are always videos the playlist did contain. It is recorded here because
a test asserting the union invariant across two accounts at once would fail, and the honest reading
of that failure is this gap, not a broken rebuild.

## Gotchas

- **Thumbnail decoding fails silently.** The `medium`/`default` fallback in both fetchers means a
  thumbnail that does not decode produces an empty string rather than an error, so a wrong JSON tag
  on the response structs degrades quality invisibly. `server/api/youtube_test.go` pins both structs
  against a representative API payload.
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
- `server/api/youtube_test.go` — decode tests pinning the response structs to the YouTube API's
  payload shape.
- `server/api/custom_handlers.go` — `customAddPlaylist` (the only caller), `customRemovePlaylist`,
  `refreshUserHasVideo` and `selectVideoIfUnavailable` (the user-pool side).
- `server/api/playlist_model.generated.go` — `Playlist` model and `playlistManager`
  (Create/Update/Get); generated from `models.json`.
- `server/common/config.go` — `YouTubeAPIKey` (`youtube_api_key`), required via `Config.Validate`.
- `docs/schema.md` — the `playlists`/`videos` tables and the `playlist_video`/`user_playlist`/
  `user_has_video` join tables.
- `docs/settings.md` — the user-facing playlist add/remove UI.

## Extension checklist (changing the sync)

1. New YouTube API field needed → add it to the `YouTubePlaylist*Response` struct with the correct
   JSON tag, and cover it in `server/api/youtube_test.go`: a mistagged field decodes to the zero
   value without erroring.
2. New DB column on `videos`/`playlists` → migration + regenerate the model from `models.json`
   (`make build-api`), then thread it through the `INSERT`/`Update` in `syncPlaylistFromYouTube`.
3. Changing membership semantics (e.g. soft-delete instead of clear-and-rebuild) → update step 4 and
   the invariants above.
4. If the YouTube host, page size, watch-URL prefix, or the key-required rule changes, update the
   DOC-SYNC anchor block (the test fails CI otherwise).
