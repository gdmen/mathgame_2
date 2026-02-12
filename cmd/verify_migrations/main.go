// verify_migrations checks that a "before" DB (migrations 1-14) and an "after" DB
// (migrations 15-23 applied) are consistent: events/gamestates/user_has_video/playlist_video
// reference valid videos, videos are de-duplicated by canonical key, and user-video links
// are correctly remapped. It does not run migrations.
//
// Usage: go run ./cmd/verify_migrations -before-config before.json -after-config after.json
package main

import (
	"database/sql"
	"flag"
	"fmt"
	"os"
	"regexp"
	"strings"

	_ "github.com/go-sql-driver/mysql"

	"garydmenezes.com/mathgame/server/common"
)

func main() {
	beforeConfig := flag.String("before-config", "", "path to config JSON for DB before migrations 15+")
	afterConfig := flag.String("after-config", "", "path to config JSON for DB after migrations 15+")
	flag.Parse()
	if *beforeConfig == "" || *afterConfig == "" {
		fmt.Fprintln(os.Stderr, "need -before-config and -after-config")
		os.Exit(1)
	}

	before, err := common.ReadConfig(*beforeConfig)
	if err != nil {
		fmt.Fprintln(os.Stderr, "before config:", err)
		os.Exit(1)
	}
	if err := before.Validate(); err != nil {
		fmt.Fprintln(os.Stderr, "before config:", err)
		os.Exit(1)
	}
	after, err := common.ReadConfig(*afterConfig)
	if err != nil {
		fmt.Fprintln(os.Stderr, "after config:", err)
		os.Exit(1)
	}
	if err := after.Validate(); err != nil {
		fmt.Fprintln(os.Stderr, "after config:", err)
		os.Exit(1)
	}

	beforeDB, err := openDB(before)
	if err != nil {
		fmt.Fprintln(os.Stderr, "before DB:", err)
		os.Exit(1)
	}
	defer beforeDB.Close()
	afterDB, err := openDB(after)
	if err != nil {
		fmt.Fprintln(os.Stderr, "after DB:", err)
		os.Exit(1)
	}
	defer afterDB.Close()

	var failed int
	fail := func(msg string) {
		failed++
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", msg)
	}
	detail := func(format string, args ...interface{}) {
		fmt.Fprintf(os.Stderr, "  "+format+"\n", args...)
	}
	ok := func(msg string) { fmt.Printf("OK: %s\n", msg) }

	// 1. After: every gamestate.video_id exists in videos
	{
		var bad int
		err := afterDB.QueryRow(`
			SELECT COUNT(*) FROM gamestates g
			LEFT JOIN videos v ON v.id = g.video_id
			WHERE v.id IS NULL`).Scan(&bad)
		if err != nil {
			fail("gamestate video refs: " + err.Error())
		} else if bad != 0 {
			fail(fmt.Sprintf("gamestate video refs: %d gamestates reference missing video", bad))
			detail("gamestates rows where video_id is not in videos table:")
			rows, _ := afterDB.Query(`SELECT g.user_id, g.problem_id, g.video_id, g.solved, g.target FROM gamestates g LEFT JOIN videos v ON v.id = g.video_id WHERE v.id IS NULL`)
			if rows != nil {
				for rows.Next() {
					var uid, problemID, vid, solved, target int
					if rows.Scan(&uid, &problemID, &vid, &solved, &target) == nil {
						detail("  user_id=%d problem_id=%d video_id=%d (missing) solved=%d target=%d", uid, problemID, vid, solved, target)
					}
				}
				rows.Close()
			}
		} else {
			ok("gamestate.video_id references exist in videos")
		}
	}

	// 2. After: every done_watching_video event value (as video id) exists in videos
	{
		var bad int
		err := afterDB.QueryRow(`
			SELECT COUNT(*) FROM events e
			LEFT JOIN videos v ON v.id = CAST(e.value AS UNSIGNED)
			WHERE e.event_type = 'done_watching_video' AND e.value REGEXP '^[0-9]+$' AND v.id IS NULL`).Scan(&bad)
		if err != nil {
			fail("events done_watching_video: " + err.Error())
		} else if bad != 0 {
			fail(fmt.Sprintf("events: %d done_watching_video events reference missing video", bad))
			rows, _ := afterDB.Query(`SELECT e.id AS event_id, e.user_id, e.value FROM events e LEFT JOIN videos v ON v.id = CAST(e.value AS UNSIGNED) WHERE e.event_type = 'done_watching_video' AND e.value REGEXP '^[0-9]+$' AND v.id IS NULL`)
			if rows != nil {
				for rows.Next() {
					var eid, uid int
					var val string
					if rows.Scan(&eid, &uid, &val) == nil {
						detail("event_id=%d user_id=%d value (video_id)=%s", eid, uid, val)
					}
				}
				rows.Close()
			}
		} else {
			ok("done_watching_video event values exist in videos")
		}
	}

	// 3. After: every user_has_video.video_id exists in videos
	{
		var bad int
		err := afterDB.QueryRow(`
			SELECT COUNT(*) FROM user_has_video uhv
			LEFT JOIN videos v ON v.id = uhv.video_id
			WHERE v.id IS NULL`).Scan(&bad)
		if err != nil {
			fail("user_has_video.video_id: " + err.Error())
		} else if bad != 0 {
			fail(fmt.Sprintf("user_has_video.video_id: %d rows reference missing video", bad))
			rows, _ := afterDB.Query(`SELECT uhv.user_id, uhv.video_id FROM user_has_video uhv LEFT JOIN videos v ON v.id = uhv.video_id WHERE v.id IS NULL`)
			if rows != nil {
				for rows.Next() {
					var uid, vid int
					if rows.Scan(&uid, &vid) == nil {
						detail("user_id=%d video_id=%d", uid, vid)
					}
				}
				rows.Close()
			}
		} else {
			ok("user_has_video.video_id references exist in videos")
		}
	}

	// 4. After: every playlist_video.video_id exists in videos
	{
		var bad int
		err := afterDB.QueryRow(`
			SELECT COUNT(*) FROM playlist_video pv
			LEFT JOIN videos v ON v.id = pv.video_id
			WHERE v.id IS NULL`).Scan(&bad)
		if err != nil {
			fail("playlist_video.video_id: " + err.Error())
		} else if bad != 0 {
			fail(fmt.Sprintf("playlist_video.video_id: %d rows reference missing video", bad))
			rows, _ := afterDB.Query(`SELECT pv.playlist_id, pv.video_id FROM playlist_video pv LEFT JOIN videos v ON v.id = pv.video_id WHERE v.id IS NULL`)
			if rows != nil {
				for rows.Next() {
					var pid, vid int
					if rows.Scan(&pid, &vid) == nil {
						detail("playlist_id=%d video_id=%d", pid, vid)
					}
				}
				rows.Close()
			}
		} else {
			ok("playlist_video.video_id references exist in videos")
		}
	}

	// 5. After: no duplicate you_tube_id in videos (each non-null you_tube_id at most one row)
	{
		var dup int
		err := afterDB.QueryRow(`
			SELECT COUNT(*) FROM (
				SELECT you_tube_id FROM videos WHERE you_tube_id IS NOT NULL
				GROUP BY you_tube_id HAVING COUNT(*) > 1
			) t`).Scan(&dup)
		if err != nil {
			fail("videos de-dup: " + err.Error())
		} else if dup != 0 {
			fail(fmt.Sprintf("videos de-dup: %d you_tube_id values have more than one row", dup))
			rows, _ := afterDB.Query(`SELECT you_tube_id, COUNT(*) AS cnt FROM videos WHERE you_tube_id IS NOT NULL GROUP BY you_tube_id HAVING COUNT(*) > 1`)
			if rows != nil {
				for rows.Next() {
					var ytID string
					var cnt int
					if rows.Scan(&ytID, &cnt) == nil {
						detail("you_tube_id=%q row_count=%d", ytID, cnt)
					}
				}
				rows.Close()
			}
		} else {
			ok("videos have at most one row per you_tube_id")
		}
	}

	// 6. Before -> After: every (user_id, video_id) from before.videos (user_id > 0) remapped to winner in after.user_has_video
	{
		rows, err := beforeDB.Query(`SELECT id, user_id, url FROM videos WHERE user_id > 0`)
		if err != nil {
			fail("before videos: " + err.Error())
		} else {
			type uv struct{ userID, videoID int }
			beforePairs := make(map[uv]string) // (user_id, video_id) -> canonical_key
			for rows.Next() {
				var id, userID int
				var url string
				if err := rows.Scan(&id, &userID, &url); err != nil {
					fail("before videos scan: " + err.Error())
					rows.Close()
					break
				}
				key := canonicalKey(url)
				beforePairs[uv{userID, id}] = key
			}
			rows.Close()
			if err := rows.Err(); err != nil {
				fail("before videos: " + err.Error())
			} else {
				// after: you_tube_id -> one video id (winner)
				keyToID := make(map[string]int)
				r, err := afterDB.Query(`SELECT id, you_tube_id FROM videos WHERE you_tube_id IS NOT NULL`)
				if err != nil {
					fail("after videos: " + err.Error())
				} else {
					for r.Next() {
						var id int
						var ytID string
						if err := r.Scan(&id, &ytID); err != nil {
							continue
						}
						keyToID[ytID] = id
					}
					r.Close()
				}
				// after: (user_id, video_id) set
				uhv := make(map[uv]bool)
				r2, err := afterDB.Query(`SELECT user_id, video_id FROM user_has_video`)
				if err != nil {
					fail("after user_has_video: " + err.Error())
				} else {
					for r2.Next() {
						var uid, vid int
						if err := r2.Scan(&uid, &vid); err != nil {
							continue
						}
						uhv[uv{uid, vid}] = true
					}
					r2.Close()
				}
				var missingPairs []string
				for p, key := range beforePairs {
					winnerID, ok := keyToID[key]
					if !ok {
						// key might be full url if not parseable; try matching by url in after
						winnerID = -1
						var aid int
						var aurl string
						rr, _ := afterDB.Query(`SELECT id, url FROM videos`)
						for rr != nil && rr.Next() {
							rr.Scan(&aid, &aurl)
							if canonicalKey(aurl) == key {
								winnerID = aid
								break
							}
						}
						if rr != nil {
							rr.Close()
						}
					}
					if winnerID < 0 {
						missingPairs = append(missingPairs, fmt.Sprintf("user_id=%d before_video_id=%d canonical_key=%q (no matching video in after)", p.userID, p.videoID, key))
						continue
					}
					if !uhv[uv{p.userID, winnerID}] {
						missingPairs = append(missingPairs, fmt.Sprintf("user_id=%d before_video_id=%d canonical_key=%q after_winner_video_id=%d (not in user_has_video)", p.userID, p.videoID, key, winnerID))
					}
				}
				if len(missingPairs) != 0 {
					fail(fmt.Sprintf("user-video remap: %d (user_id, video) from before missing in after user_has_video", len(missingPairs)))
					for _, s := range missingPairs {
						detail("%s", s)
					}
				} else {
					ok("user-video links remapped to winners in user_has_video")
				}
			}
		}
	}

	if failed != 0 {
		os.Exit(1)
	}
}

func openDB(c *common.Config) (*sql.DB, error) {
	conn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=true",
		c.MySQLUser, c.MySQLPass, c.MySQLHost, c.MySQLPort, c.MySQLDatabase)
	return sql.Open("mysql", conn)
}

// canonicalKey matches migration 22: youtu.be/ID, or v=ID, else url.
func canonicalKey(url string) string {
	if strings.Contains(url, "youtu.be/") {
		parts := strings.Split(url, "/")
		s := parts[len(parts)-1]
		if len(s) > 11 {
			return s[:11]
		}
		return s
	}
	if strings.Contains(url, "v=") {
		re := regexp.MustCompile(`[?&]v=([^&]+)`)
		m := re.FindStringSubmatch(url)
		if len(m) >= 2 {
			s := m[1]
			if len(s) > 11 {
				return s[:11]
			}
			return s
		}
	}
	return url
}
