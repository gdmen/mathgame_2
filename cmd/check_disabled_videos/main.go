// check_disabled_videos lists videos with disabled=1 and checks if each can be played
// (exists, public, embeddable) via YouTube Data API v3 or oembed fallback.
package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"garydmenezes.com/mathgame/server/common"
)

const (
	queryDisabledVideos = `SELECT id, title, url, thumbnailurl, you_tube_id, disabled FROM videos WHERE disabled = 1`
	updateVideoEnabled  = `UPDATE videos SET disabled=0 WHERE id=?`
	youTubeVideosURL    = "https://www.googleapis.com/youtube/v3/videos"
	youTubeOembedURL    = "https://www.youtube.com/oembed"
	batchSize           = 50
	httpTimeout         = 15 * time.Second
)

type videoRow struct {
	Id           uint32
	Title        string
	URL          string
	ThumbnailURL string
	YouTubeId    string
	Disabled     bool
}

type youTubeVideosResponse struct {
	Items []struct {
		Id     string `json:"id"`
		Status struct {
			Embeddable      bool   `json:"embeddable"`
			PrivacyStatus   string `json:"privacyStatus"`
			UploadStatus    string `json:"uploadStatus"`
			RejectionReason string `json:"rejectionReason"`
		} `json:"status"`
	} `json:"items"`
}

type result string

const (
	resultPlayable    result = "playable"
	resultNotPlayable result = "not playable"
	resultError       result = "error"
	resultNoID        result = "no YouTube ID"
)

func main() {
	configPath := flag.String("config", "conf.json", "path to config JSON")
	enable := flag.Bool("enable", false, "run UPDATE to set disabled=0 for videos that are playable")
	flag.Parse()

	c, err := common.ReadConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read config: %v\n", err)
		os.Exit(1)
	}

	connStr := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=true&time_zone=UTC",
		c.MySQLUser, c.MySQLPass, c.MySQLHost, c.MySQLPort, c.MySQLDatabase)
	db, err := sql.Open("mysql", connStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "connect: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		fmt.Fprintf(os.Stderr, "ping db: %v\n", err)
		os.Exit(1)
	}

	rows, err := db.Query(queryDisabledVideos)
	if err != nil {
		fmt.Fprintf(os.Stderr, "query disabled videos: %v\n", err)
		os.Exit(1)
	}
	defer rows.Close()

	var videos []videoRow
	for rows.Next() {
		var v videoRow
		var thumb string
		if err := rows.Scan(&v.Id, &v.Title, &v.URL, &thumb, &v.YouTubeId, &v.Disabled); err != nil {
			fmt.Fprintf(os.Stderr, "scan row: %v\n", err)
			os.Exit(1)
		}
		v.ThumbnailURL = thumb
		videos = append(videos, v)
	}
	if err := rows.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "rows: %v\n", err)
		os.Exit(1)
	}

	if len(videos) == 0 {
		fmt.Println("No disabled videos found.")
		return
	}

	client := &http.Client{Timeout: httpTimeout}
	playableByID := make(map[string]result)

	if strings.TrimSpace(c.YouTubeAPIKey) != "" {
		checkViaYouTubeAPI(client, c.YouTubeAPIKey, videos, playableByID)
	} else {
		fmt.Fprintln(os.Stderr, "No youtube_api_key in config; using oembed fallback (best-effort only).")
		checkViaOembed(client, videos, playableByID)
	}

	fmt.Printf("%d disabled video(s) checked:\n", len(videos))
	fmt.Println("id\ttitle\tyou_tube_id\tresult")
	for _, v := range videos {
		res := resultNoID
		if v.YouTubeId != "" {
			res = playableByID[v.YouTubeId]
			if res == "" {
				res = resultError
			}
		}
		title := v.Title
		if len(title) > 50 {
			title = title[:47] + "..."
		}
		fmt.Printf("%d\t%s\t%s\t%s\n", v.Id, title, v.YouTubeId, res)
	}

	if *enable {
		for _, v := range videos {
			if v.YouTubeId == "" || playableByID[v.YouTubeId] != resultPlayable {
				continue
			}
			_, err := db.Exec(updateVideoEnabled, v.Id)
			if err != nil {
				fmt.Fprintf(os.Stderr, "update video id %d: %v\n", v.Id, err)
				continue
			}
			fmt.Printf("Enabled video id %d (%s)\n", v.Id, v.Title)
		}
	}
}

func checkViaYouTubeAPI(client *http.Client, apiKey string, videos []videoRow, out map[string]result) {
	idsWithVideo := make([]string, 0, len(videos))
	for _, v := range videos {
		if strings.TrimSpace(v.YouTubeId) == "" {
			continue
		}
		idsWithVideo = append(idsWithVideo, v.YouTubeId)
	}

	for i := 0; i < len(idsWithVideo); i += batchSize {
		end := i + batchSize
		if end > len(idsWithVideo) {
			end = len(idsWithVideo)
		}
		batch := idsWithVideo[i:end]
		idsParam := strings.Join(batch, ",")
		reqURL := fmt.Sprintf("%s?part=status&id=%s&key=%s",
			youTubeVideosURL, url.QueryEscape(idsParam), url.QueryEscape(apiKey))

		resp, err := client.Get(reqURL)
		if err != nil {
			for _, id := range batch {
				out[id] = resultError
			}
			fmt.Fprintf(os.Stderr, "YouTube API request: %v\n", err)
			continue
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			for _, id := range batch {
				out[id] = resultError
			}
			fmt.Fprintf(os.Stderr, "read response: %v\n", err)
			continue
		}
		if resp.StatusCode != http.StatusOK {
			for _, id := range batch {
				out[id] = resultError
			}
			fmt.Fprintf(os.Stderr, "YouTube API %d: %s\n", resp.StatusCode, string(body))
			continue
		}

		var data youTubeVideosResponse
		if err := json.Unmarshal(body, &data); err != nil {
			for _, id := range batch {
				out[id] = resultError
			}
			fmt.Fprintf(os.Stderr, "decode response: %v\n", err)
			continue
		}

		found := make(map[string]bool)
		for _, item := range data.Items {
			found[item.Id] = true
			playable := item.Status.Embeddable && item.Status.PrivacyStatus == "public"
			if playable {
				out[item.Id] = resultPlayable
			} else {
				out[item.Id] = resultNotPlayable
			}
		}
		for _, id := range batch {
			if !found[id] {
				out[id] = resultNotPlayable
			}
		}
	}
}

func checkViaOembed(client *http.Client, videos []videoRow, out map[string]result) {
	for _, v := range videos {
		if strings.TrimSpace(v.YouTubeId) == "" {
			continue
		}
		watchURL := "https://www.youtube.com/watch?v=" + v.YouTubeId
		reqURL := youTubeOembedURL + "?url=" + url.QueryEscape(watchURL)

		resp, err := client.Get(reqURL)
		if err != nil {
			out[v.YouTubeId] = resultError
			fmt.Fprintf(os.Stderr, "oembed %s: %v\n", v.YouTubeId, err)
			continue
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			out[v.YouTubeId] = resultPlayable
		} else {
			out[v.YouTubeId] = resultNotPlayable
		}
	}
}
