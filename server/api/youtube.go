package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/golang/glog"
)

type YouTubePlaylistResponse struct {
	Items []struct {
		Snippet struct {
			Title      string `json:"title"`
			Thumbnails struct {
				Default struct {
					URL string `json:"url"`
				} `json:"default"`
				Medium struct {
					URL string `json:"medium"`
				} `json:"medium"`
			} `json:"thumbnails"`
		} `json:"snippet"`
		Etag string `json:"etag"`
	} `json:"items"`
}

type YouTubePlaylistItemsResponse struct {
	Items []struct {
		Snippet struct {
			Title      string `json:"title"`
			ResourceID struct {
				VideoID string `json:"videoId"`
			} `json:"resourceId"`
			Thumbnails struct {
				Default struct {
					URL string `json:"url"`
				} `json:"default"`
				Medium struct {
					URL string `json:"medium"`
				} `json:"medium"`
			} `json:"thumbnails"`
		} `json:"snippet"`
	} `json:"items"`
	NextPageToken string `json:"nextPageToken"`
}

func (a *Api) fetchPlaylistMetadata(playlistID string) (thumbURL, etag, title string, err error) {
	apiURL := fmt.Sprintf("https://www.googleapis.com/youtube/v3/playlists?part=snippet&id=%s&key=%s",
		url.QueryEscape(playlistID), url.QueryEscape(a.YouTubeAPIKey))
	resp, err := http.Get(apiURL)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to fetch playlist: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", "", "", fmt.Errorf("YouTube API error: %d %s", resp.StatusCode, string(body))
	}
	var data YouTubePlaylistResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return "", "", "", fmt.Errorf("failed to decode response: %w", err)
	}
	if len(data.Items) == 0 {
		return "", "", "", fmt.Errorf("playlist not found")
	}
	snippet := data.Items[0].Snippet
	thumbURL = snippet.Thumbnails.Medium.URL
	if thumbURL == "" {
		thumbURL = snippet.Thumbnails.Default.URL
	}
	return thumbURL, data.Items[0].Etag, snippet.Title, nil
}

func (a *Api) fetchPlaylistItems(playlistID string) ([]struct {
	VideoID      string
	Title        string
	ThumbnailURL string
}, error) {
	var allItems []struct {
		VideoID      string
		Title        string
		ThumbnailURL string
	}
	pageToken := ""
	for {
		apiURL := fmt.Sprintf("https://www.googleapis.com/youtube/v3/playlistItems?part=snippet&playlistId=%s&maxResults=50&key=%s",
			url.QueryEscape(playlistID), url.QueryEscape(a.YouTubeAPIKey))
		if pageToken != "" {
			apiURL += "&pageToken=" + url.QueryEscape(pageToken)
		}
		resp, err := http.Get(apiURL)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch playlist items: %w", err)
		}
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return nil, fmt.Errorf("YouTube API error: %d %s", resp.StatusCode, string(body))
		}
		var data YouTubePlaylistItemsResponse
		if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("failed to decode playlist items: %w", err)
		}
		resp.Body.Close()
		for _, item := range data.Items {
			thumbURL := item.Snippet.Thumbnails.Medium.URL
			if thumbURL == "" {
				thumbURL = item.Snippet.Thumbnails.Default.URL
			}
			allItems = append(allItems, struct {
				VideoID      string
				Title        string
				ThumbnailURL string
			}{
				VideoID:      item.Snippet.ResourceID.VideoID,
				Title:        item.Snippet.Title,
				ThumbnailURL: thumbURL,
			})
		}
		if data.NextPageToken == "" {
			break
		}
		pageToken = data.NextPageToken
	}
	return allItems, nil
}

func (a *Api) syncPlaylistFromYouTube(playlistID string) (uint32, error) {
	thumbURL, etag, title, err := a.fetchPlaylistMetadata(playlistID)
	if err != nil {
		return 0, fmt.Errorf("fetch playlist metadata: %w", err)
	}
	var playlistDbID uint32
	err = a.DB.QueryRow("SELECT id FROM playlists WHERE you_tube_id=?", playlistID).Scan(&playlistDbID)
	if err == sql.ErrNoRows {
		pl := &Playlist{
			YouTubeId:    playlistID,
			Title:        title,
			ThumbnailURL: thumbURL,
			Etag:         etag,
		}
		status, msg, createErr := a.playlistManager.Create(pl)
		if createErr != nil {
			return 0, fmt.Errorf("create playlist: %d %s: %w", status, msg, createErr)
		}
		playlistDbID = pl.Id
	} else if err != nil {
		return 0, fmt.Errorf("query playlist: %w", err)
	} else {
		pl := &Playlist{
			Id:           playlistDbID,
			YouTubeId:    playlistID,
			Title:        title,
			ThumbnailURL: thumbURL,
			Etag:         etag,
		}
		status, msg, updateErr := a.playlistManager.Update(pl)
		if updateErr != nil {
			return 0, fmt.Errorf("update playlist: %d %s: %w", status, msg, updateErr)
		}
	}
	items, err := a.fetchPlaylistItems(playlistID)
	if err != nil {
		return 0, fmt.Errorf("fetch playlist items: %w", err)
	}
	_, err = a.DB.Exec("DELETE FROM playlist_video WHERE playlist_id=?", playlistDbID)
	if err != nil {
		return 0, fmt.Errorf("clear playlist_video: %w", err)
	}
	for _, item := range items {
		var videoID uint32
		qerr := a.DB.QueryRow("SELECT id FROM videos WHERE you_tube_id=?", item.VideoID).Scan(&videoID)
		if qerr == sql.ErrNoRows {
			videoURL := "https://www.youtube.com/watch?v=" + item.VideoID
			result, inerr := a.DB.Exec("INSERT INTO videos (title, url, thumbnailurl, you_tube_id) VALUES (?, ?, ?, ?)",
				item.Title, videoURL, item.ThumbnailURL, item.VideoID)
			if inerr != nil {
				glog.Errorf("insert video %s: %v", item.VideoID, inerr)
				continue
			}
			id, inerr := result.LastInsertId()
			if inerr != nil {
				glog.Errorf("get video insert id %s: %v", item.VideoID, inerr)
				continue
			}
			videoID = uint32(id)
		} else if qerr != nil {
			glog.Errorf("query video %s: %v", item.VideoID, qerr)
			continue
		}
		_, err = a.DB.Exec("INSERT INTO playlist_video (playlist_id, video_id) VALUES (?, ?)", playlistDbID, videoID)
		if err != nil {
			if !strings.Contains(err.Error(), "Duplicate entry") {
				glog.Errorf("insert playlist_video %d %d: %v", playlistDbID, videoID, err)
			}
		}
	}
	return playlistDbID, nil
}
