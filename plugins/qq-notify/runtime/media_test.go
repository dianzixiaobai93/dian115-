package main

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

func TestIndependentTestSendsDoNotCollide(t *testing.T) {
	m := mock()
	r := Runtime{host: m}
	c := configForTest()
	saveConfig(m, c)
	for _, id := range []string{"first", "second"} {
		result, err := action(&r, id, "send-test", nil)
		if err != nil || result["status"] != "succeeded" {
			t.Fatal(result, err)
		}
	}
	if len(m.sent) != 2 {
		t.Fatal("independent tests not sent", len(m.sent))
	}
}

func TestMediaImageAndMissingFields(t *testing.T) {
	m := mock()
	r := Runtime{host: m}
	i := Item{ID: "42", Name: "Example", Year: 2026, Type: "Movie", Providers: map[string]string{"Tmdb": "123"}}
	m.tmdb = map[string]any{"backdrop_path": "/example.jpg", "vote_average": 8.1, "genres": []any{map[string]any{"name": "Drama"}}}
	n := r.mediaNotice(configForTest(), i)
	if n.Image != "https://image.tmdb.org/t/p/w780/example.jpg" || !strings.Contains(n.Body, "8.1/10") {
		t.Fatal(n)
	}
	if err := r.send(configForTest().Destination, render(configForTest(), n), n.Image); err != nil {
		t.Fatal(err)
	}
	b, _ := base64.RawStdEncoding.DecodeString(m.sent[0].Body)
	var p struct {
		Message []struct {
			Type string `json:"type"`
		} `json:"message"`
	}
	if json.Unmarshal(b, &p) != nil || len(p.Message) != 2 || p.Message[0].Type != "image" || p.Message[1].Type != "text" {
		t.Fatal(string(b))
	}
	m.tmdb = map[string]any{}
	n = r.mediaNotice(configForTest(), i)
	if n.Image != "" || strings.Contains(n.Body, "评分") || strings.Contains(n.Body, "大小") {
		t.Fatal("invented unavailable metadata", n)
	}
	if tmdbImage("/../secret.jpg", "w780") != "" || len(messageSegments("literal [CQ:at,qq=all]", []string{"http://example.invalid/image?token=secret"})) != 1 {
		t.Fatal("unsafe image forwarded")
	}
	m.tmdb = map[string]any{"backdrop_path": "/wrong-series.jpg"}
	n = r.mediaNotice(configForTest(), Item{ID: "43", Type: "Episode", SeriesID: "missing", Providers: map[string]string{"Tmdb": "456"}})
	if n.Image != "" {
		t.Fatal("episode ID incorrectly used as TV series ID", n)
	}
}
