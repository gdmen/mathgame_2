package api

import (
	"encoding/json"
	"testing"
)

const testMediumThumbURL = "https://i.ytimg.com/vi/abc123/mqdefault.jpg"
const testDefaultThumbURL = "https://i.ytimg.com/vi/abc123/default.jpg"

func TestPlaylistResponseDecodesMediumThumbnail(t *testing.T) {
	body := `{"items":[{"etag":"E1","snippet":{"title":"PL","thumbnails":{
		"default":{"url":"` + testDefaultThumbURL + `","width":120,"height":90},
		"medium":{"url":"` + testMediumThumbURL + `","width":320,"height":180}}}}]}`

	var data YouTubePlaylistResponse
	if err := json.Unmarshal([]byte(body), &data); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(data.Items) != 1 {
		t.Fatalf("got %d items, want 1", len(data.Items))
	}
	thumbs := data.Items[0].Snippet.Thumbnails
	if thumbs.Medium.URL != testMediumThumbURL {
		t.Errorf("Medium.URL = %q, want %q", thumbs.Medium.URL, testMediumThumbURL)
	}
	if thumbs.Default.URL != testDefaultThumbURL {
		t.Errorf("Default.URL = %q, want %q", thumbs.Default.URL, testDefaultThumbURL)
	}
}

func TestPlaylistItemsResponseDecodesMediumThumbnail(t *testing.T) {
	body := `{"items":[{"snippet":{"title":"V","resourceId":{"videoId":"abc123"},"thumbnails":{
		"default":{"url":"` + testDefaultThumbURL + `","width":120,"height":90},
		"medium":{"url":"` + testMediumThumbURL + `","width":320,"height":180}}}}],
		"nextPageToken":""}`

	var data YouTubePlaylistItemsResponse
	if err := json.Unmarshal([]byte(body), &data); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(data.Items) != 1 {
		t.Fatalf("got %d items, want 1", len(data.Items))
	}
	snippet := data.Items[0].Snippet
	if snippet.ResourceID.VideoID != "abc123" {
		t.Errorf("VideoID = %q, want %q", snippet.ResourceID.VideoID, "abc123")
	}
	if snippet.Thumbnails.Medium.URL != testMediumThumbURL {
		t.Errorf("Medium.URL = %q, want %q", snippet.Thumbnails.Medium.URL, testMediumThumbURL)
	}
	if snippet.Thumbnails.Default.URL != testDefaultThumbURL {
		t.Errorf("Default.URL = %q, want %q", snippet.Thumbnails.Default.URL, testDefaultThumbURL)
	}
}
