package main

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Only public TMDB images are forwarded; no host credentials or local paths.
func tmdbImage(path, size string) string {
	name := strings.TrimPrefix(path, "/")
	if name == "" || !strings.HasPrefix(path, "/") || strings.Contains(name, "..") || url.PathEscape(name) != name {
		return ""
	}
	return "https://image.tmdb.org/t/p/" + size + "/" + name
}

func messageSegments(text string, images []string) []map[string]any {
	segments := []map[string]any{}
	for _, image := range images {
		if strings.HasPrefix(image, "https://image.tmdb.org/t/p/") {
			u, err := url.Parse(image)
			if err == nil && u.Host == "image.tmdb.org" && u.User == nil && u.RawQuery == "" && u.Fragment == "" {
				segments = append(segments, map[string]any{"type": "image", "data": map[string]string{"file": image}})
				break
			}
		}
	}
	return append(segments, map[string]any{"type": "text", "data": map[string]string{"text": text}})
}

func (r *Runtime) mediaItems(c Config, typ, query string, limit int) ([]Item, error) {
	q := url.Values{"type": {typ}, "limit": {strconv.Itoa(limit)}, "offset": {"0"}, "sort_by": {"date_created"}, "sort_order": {"desc"}}
	if c.ProxyID != nil && *c.ProxyID > 0 {
		q.Set("proxy_id", strconv.Itoa(*c.ProxyID))
	}
	if c.Library != "" {
		q.Set("library_id", c.Library)
	}
	if query != "" {
		q.Set("q", query)
	}
	resp, err := r.request(Request{Method: "GET", Path: "/api/plugin-host/emby/items?" + q.Encode()})
	if err != nil {
		return nil, err
	}
	var env struct {
		Items []Item `json:"items"`
	}
	if err = decode(resp, &env); err != nil {
		return nil, err
	}
	return env.Items, nil
}

func (r *Runtime) mediaNotice(c Config, item Item) Notice {
	meta := item
	kind, typ := "电影", "movie"
	name := item.Name
	if item.Type == "Episode" {
		// Episode TMDB IDs cannot be used to look up a TV series.
		meta.Providers = nil
		kind, typ, name = "电视剧", "tv", item.Series
		if parents, err := r.mediaItems(c, "Series", item.Series, 50); err == nil {
			for _, parent := range parents {
				if parent.ID == item.SeriesID && item.SeriesID != "" {
					meta = parent
					break
				}
			}
		}
		if name == "" {
			name = item.Name
		}
	}
	image := ""
	countries := []string{}
	var detail struct {
		Backdrop  string   `json:"backdrop_path"`
		Poster    string   `json:"poster_path"`
		Rating    float64  `json:"vote_average"`
		Origin    []string `json:"origin_country"`
		Countries []struct {
			Name string `json:"name"`
		} `json:"production_countries"`
		Genres []struct {
			Name string `json:"name"`
		} `json:"genres"`
	}
	tmdb := ""
	for key, value := range meta.Providers {
		if strings.EqualFold(key, "tmdb") {
			tmdb = value
		}
	}
	if id, err := strconv.ParseInt(tmdb, 10, 64); err == nil && id > 0 {
		resp, err := r.request(Request{Method: "GET", Path: "/api/tmdb/" + typ + "/" + strconv.FormatInt(id, 10)})
		if err == nil && decode(resp, &detail) == nil {
			image = tmdbImage(detail.Backdrop, "w780")
			if image == "" {
				image = tmdbImage(detail.Poster, "w500")
			}
			if meta.Rating == 0 {
				meta.Rating = detail.Rating
			}
			if len(meta.Genres) == 0 {
				for _, genre := range detail.Genres {
					if genre.Name != "" {
						meta.Genres = append(meta.Genres, genre.Name)
					}
				}
			}
			for _, country := range detail.Countries {
				if country.Name != "" {
					countries = append(countries, country.Name)
				}
			}
			if len(countries) == 0 {
				countries = detail.Origin
			}
		}
	}
	title := "🎬 " + name
	if meta.Year > 0 {
		title += fmt.Sprintf(" (%d)", meta.Year)
	}
	if item.Type == "Episode" {
		title += fmt.Sprintf(" S%02d E%02d", item.Season, item.Episode)
	}
	title += " 已入库"
	lines := []string{}
	if meta.Rating > 0 && meta.Rating <= 10 {
		lines = append(lines, fmt.Sprintf("⭐ 评分：%.1f/10", meta.Rating))
	}
	lines = append(lines, "📚 类型："+kind)
	if len(meta.Genres) > 0 {
		lines = append(lines, "🗂 分类："+strings.Join(meta.Genres, " / "))
	}
	if len(countries) > 0 {
		lines = append(lines, "🌍 地区："+strings.Join(countries, " / "))
	}
	lines = append(lines, "🎞 Emby 编号："+item.ID)
	return Notice{Title: clip(title, 256), Body: clip(strings.Join(lines, "\n"), 1500), Level: "success", Image: image, At: item.Date}
}

func oneBotHTTPError(status int) error {
	switch status {
	case 401, 403:
		return fmt.Errorf("NapCat HTTP %d：鉴权失败，请核对 HTTP Server Token 和托管凭据引用（不是 WebUI Token）", status)
	case 502:
		return fmt.Errorf("NapCat HTTP 502：上游不可用，请检查 QQ 登录状态、HTTP 端口和网络")
	default:
		return fmt.Errorf("NapCat HTTP %d", status)
	}
}

func (r *Runtime) previewNotice(c Config) (Notice, bool) {
	for _, typ := range c.Types {
		items, err := r.mediaItems(c, typ, "", 1)
		if err == nil && len(items) > 0 {
			n := r.mediaNotice(c, items[0])
			n.Title = "【测试预览】" + n.Title
			n.At = time.Now().UTC().Format(time.RFC3339)
			return n, true
		}
	}
	return Notice{}, false
}
