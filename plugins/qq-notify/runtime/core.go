package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

type Host interface{ Call(string, any, any) error }
type Runtime struct{ host Host }
type RPC struct {
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}
type Request struct {
	Method     string            `json:"method"`
	Path       string            `json:"path"`
	Headers    map[string]string `json:"headers,omitempty"`
	Body       string            `json:"body_base64,omitempty"`
	Credential string            `json:"credential_ref,omitempty"`
}
type Response struct {
	Status  int                 `json:"status"`
	Headers map[string][]string `json:"headers"`
	Body    string              `json:"body_base64"`
}
type Destination struct {
	URL        string `json:"url"`
	Credential string `json:"credential_ref"`
	Kind       string `json:"target_type"`
	ID         string `json:"target_id"`
}
type Adapter struct {
	Key    string            `json:"key"`
	Path   string            `json:"path"`
	List   string            `json:"list_pointer"`
	ID     string            `json:"id_pointer"`
	Status string            `json:"status_pointer"`
	Title  string            `json:"title_pointer"`
	Body   string            `json:"body_pointer"`
	Levels map[string]string `json:"levels"`
}
type Config struct {
	Destination
	Enabled     bool      `json:"enabled"`
	Prefix      string    `json:"prefix"`
	Template    string    `json:"template"`
	Levels      []string  `json:"levels"`
	Interval    int       `json:"interval_seconds"`
	MaxAttempts int       `json:"max_attempts"`
	Emby        bool      `json:"emby_enabled"`
	ProxyID     *int      `json:"proxy_id"`
	Library     string    `json:"library_id"`
	Types       []string  `json:"media_types"`
	Adapters    []Adapter `json:"adapters"`
}

func defaults() Config {
	return Config{Destination: Destination{URL: "http://napcat:3000", Kind: "private"}, Prefix: "[DIAN115]", Template: "{prefix} {title}\n{body}\n{time}", Levels: []string{"success", "failed", "warning", "info"}, Interval: 60, MaxAttempts: 3, Types: []string{"Movie", "Episode"}, Adapters: []Adapter{}}
}
func allowedLevel(s string) bool {
	return s == "success" || s == "failed" || s == "warning" || s == "info"
}
func validateDestination(d Destination) error {
	u, err := url.Parse(d.URL)
	if !strings.HasPrefix(d.URL, "http://") && !strings.HasPrefix(d.URL, "https://") || len(d.URL) > 2048 || err != nil || u == nil || !(u.Scheme == "http" || u.Scheme == "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" || u.Path != "" && u.Path != "/" {
		return errors.New("NapCat 地址必须是 HTTP/HTTPS origin，例如 http://192.168.1.10:3000")
	}
	if d.Kind != "private" && d.Kind != "group" {
		return errors.New("请选择私聊或群聊")
	}
	id, err := strconv.ParseInt(d.ID, 10, 64)
	if err != nil || id <= 0 || strconv.FormatInt(id, 10) != d.ID {
		return errors.New("QQ/群号必须为正整数，不带空格或前导零")
	}
	if len(d.Credential) > 160 || strings.ContainsAny(d.Credential, "\r\n") {
		return errors.New("托管凭据引用格式无效")
	}
	return nil
}
func (c *Config) Validate() error {
	c.URL = strings.TrimRight(strings.TrimSpace(c.URL), "/")
	c.ID = strings.TrimSpace(c.ID)
	c.Credential = strings.TrimSpace(c.Credential)
	if err := validateDestination(c.Destination); err != nil {
		return err
	}
	if len(c.Library) > 256 || len(c.Levels) > 4 {
		return errors.New("媒体库 ID 或通知级别配置过长")
	}
	if c.Interval < 30 || c.Interval > 3600 {
		return errors.New("轮询间隔应为 30–3600 秒")
	}
	if c.MaxAttempts < 1 || c.MaxAttempts > 5 {
		return errors.New("总尝试次数应为 1–5")
	}
	if len(c.Prefix) > 160 || len(c.Template) > 2048 || len(c.Template) == 0 {
		return errors.New("前缀或模板长度无效")
	}
	for _, l := range c.Levels {
		if !allowedLevel(l) {
			return errors.New("通知级别无效")
		}
	}
	if c.Emby && (c.ProxyID == nil || *c.ProxyID < 0) {
		return errors.New("请明确选择 Emby 实例，旧单实例选择 ID 0")
	}
	if len(c.Types) == 0 || len(c.Types) > 2 {
		return errors.New("请选择电影或剧集")
	}
	for _, t := range c.Types {
		if t != "Movie" && t != "Episode" {
			return errors.New("仅支持 Movie 和 Episode")
		}
	}
	if len(c.Adapters) > 3 {
		return errors.New("历史适配器最多 3 个")
	}
	keys := map[string]bool{}
	for _, a := range c.Adapters {
		if a.Key == "" || len(a.Key) > 40 || keys[a.Key] || len(a.Path) > 2048 || len(a.Levels) > 20 || !safeHistoryPath(a.Path) {
			return errors.New("适配器标识重复、过长或接口不在允许列表中")
		}
		keys[a.Key] = true
		for _, s := range []string{a.List, a.ID, a.Status, a.Title} {
			if len(s) > 160 || !validPointer(s) {
				return errors.New("适配器字段必须使用 JSON Pointer，例如 /data/items 或 /id")
			}
		}
		if len(a.Body) > 160 || a.Body != "" && !validPointer(a.Body) {
			return errors.New("正文字段必须使用 JSON Pointer")
		}
		if len(a.Levels) == 0 {
			return errors.New("必须显式配置终态到通知级别的映射")
		}
		for status, l := range a.Levels {
			if len(status) > 80 || !allowedLevel(l) {
				return errors.New("适配器级别映射无效")
			}
		}
	}
	return nil
}
func safeHistoryPath(s string) bool {
	u, e := url.Parse(s)
	if e != nil || u.IsAbs() || u.Host != "" || u.Fragment != "" {
		return false
	}
	return u.Path == "/api/organize/history" || u.Path == "/api/tasks" || u.Path == "/api/history"
}
func validPointer(s string) bool {
	return s == "" || strings.HasPrefix(s, "/") && !strings.ContainsAny(s, "\r\n")
}
func body(v any) string { b, _ := json.Marshal(v); return base64.RawStdEncoding.EncodeToString(b) }
func decode(resp Response, target any) error {
	for k, v := range resp.Headers {
		if strings.EqualFold(k, "x-dian115-body-truncated") && len(v) > 0 && v[0] == "true" {
			return errors.New("宿主返回被截断，未更新增量游标")
		}
	}
	b, e := base64.RawStdEncoding.DecodeString(strings.TrimRight(resp.Body, "="))
	if e != nil {
		return errors.New("宿主正文不是有效 Base64")
	}
	if resp.Status < 200 || resp.Status >= 300 {
		return fmt.Errorf("HTTP %d", resp.Status)
	}
	decoder := json.NewDecoder(bytes.NewReader(b))
	decoder.UseNumber()
	if e = decoder.Decode(target); e != nil {
		return errors.New("接口未返回有效 JSON")
	}
	return nil
}
func (r *Runtime) request(req Request) (Response, error) {
	var resp Response
	err := r.host.Call("host.call", req, &resp)
	return resp, err
}
func header(h map[string][]string, k string) string {
	for n, v := range h {
		if strings.EqualFold(n, k) && len(v) > 0 {
			return v[0]
		}
	}
	return ""
}
func (r *Runtime) read(key string, target any) (string, bool, error) {
	resp, err := r.request(Request{Method: "GET", Path: "/api/plugin-runtime/storage/" + key})
	if err != nil {
		return "", false, err
	}
	if resp.Status == 404 {
		return "", false, nil
	}
	var env struct {
		Data struct {
			Value json.RawMessage `json:"value"`
		} `json:"data"`
	}
	if err = decode(resp, &env); err != nil {
		return "", false, err
	}
	if len(env.Data.Value) == 0 {
		return "", false, errors.New("Host Storage 缺少 data.value")
	}
	return header(resp.Headers, "etag"), true, json.Unmarshal(env.Data.Value, target)
}
func (r *Runtime) write(key, etag string, v any) error {
	b := body(map[string]any{"value": v})
	hash := sha256.Sum256([]byte(key + ":" + etag + ":" + b))
	h := map[string]string{"content-type": "application/json", "idempotency-key": "qq-store-" + hex.EncodeToString(hash[:])}
	if etag != "" {
		h["if-match"] = etag
	}
	resp, err := r.request(Request{Method: "PUT", Path: "/api/plugin-runtime/storage/" + key, Headers: h, Body: b})
	if err != nil {
		return err
	}
	if resp.Status < 200 || resp.Status >= 300 {
		return fmt.Errorf("持久化失败（HTTP %d），未确认发送状态", resp.Status)
	}
	return nil
}
func (r *Runtime) config() (Config, string, error) {
	c := defaults()
	etag, _, err := r.read("config", &c)
	return c, etag, err
}

type Notice struct {
	Image string `json:"image,omitempty"`
	ID    string `json:"id"`
	Level string `json:"level"`
	Title string `json:"title"`
	Body  string `json:"body"`
	At    string `json:"at"`
}
type Pending struct {
	Notice
	Destination
	Message   string `json:"message"`
	Attempts  int    `json:"attempts"`
	Next      int64  `json:"next"`
	Dead      bool   `json:"dead"`
	LastError string `json:"last_error"`
}
type Log struct {
	At      string `json:"at"`
	Title   string `json:"title"`
	Status  string `json:"status"`
	Message string `json:"message"`
}
type Cursor struct {
	Scope string   `json:"scope"`
	At    string   `json:"at"`
	IDs   []string `json:"ids"`
	Ready bool     `json:"ready"`
}
type AdapterCursor struct {
	Scope string   `json:"scope"`
	Seen  []string `json:"seen"`
	Ready bool     `json:"ready"`
}
type Engine struct {
	Cursors     map[string]Cursor        `json:"cursors"`
	Adapters    map[string]AdapterCursor `json:"adapters"`
	Queue       []Pending                `json:"queue"`
	Logs        []Log                    `json:"logs"`
	LastPoll    int64                    `json:"last_poll"`
	LastCommand string                   `json:"last_command"`
	LastError   string                   `json:"last_error"`
	Delivered   int                      `json:"delivered"`
}

func newEngine() Engine {
	return Engine{Cursors: map[string]Cursor{}, Adapters: map[string]AdapterCursor{}, Queue: []Pending{}, Logs: []Log{}}
}
func (e *Engine) log(status, title, message string) {
	e.Logs = append([]Log{{At: time.Now().UTC().Format(time.RFC3339), Title: safeDisplay(clip(title, 256)), Status: status, Message: safeDisplay(clip(message, 512))}}, e.Logs...)
	if len(e.Logs) > 64 {
		e.Logs = e.Logs[:64]
	}
}
func clip(s string, n int) string {
	v := []rune(s)
	if len(v) > n {
		return string(v[:n]) + "…"
	}
	return s
}
func has(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
func render(c Config, n Notice) string {
	v := strings.NewReplacer("{prefix}", c.Prefix, "{title}", n.Title, "{body}", n.Body, "{level}", n.Level, "{time}", n.At).Replace(c.Template)
	return clip(v, 3500)
}
func (e *Engine) enqueue(c Config, n Notice) error {
	if !has(c.Levels, n.Level) {
		return nil
	}
	for _, q := range e.Queue {
		if q.Notice.ID == n.ID {
			return nil
		}
	}
	if len(e.Queue) >= 200 {
		return errors.New("发送队列达到 200 条，暂停推进游标；请先处理失败队列")
	}
	e.Queue = append(e.Queue, Pending{Notice: n, Destination: c.Destination, Message: render(c, n)})
	return nil
}

type sendFailure struct {
	message string
	retry   bool
}

func (s *sendFailure) Error() string { return s.message }
func (r *Runtime) send(d Destination, message string, images ...string) error {
	if err := validateDestination(d); err != nil {
		return err
	}
	id, _ := strconv.ParseInt(d.ID, 10, 64)
	kind, key := "send_private_msg", "user_id"
	if d.Kind == "group" {
		kind, key = "send_group_msg", "group_id"
	}
	req := Request{Method: "POST", Path: d.URL + "/" + kind, Credential: d.Credential, Headers: map[string]string{"content-type": "application/json"}, Body: body(map[string]any{key: id, "message": messageSegments(message, images)})}
	resp, err := r.request(req)
	if err != nil {
		return &sendFailure{"宿主 Broker 调用失败，请检查连接和托管凭据", true}
	}
	if resp.Status < 200 || resp.Status >= 300 {
		return &sendFailure{oneBotHTTPError(resp.Status).Error(), resp.Status == 429 || resp.Status >= 500}
	}
	var env struct {
		Status string `json:"status"`
		Code   *int   `json:"retcode"`
		Data   struct {
			MessageID json.RawMessage `json:"message_id"`
		} `json:"data"`
	}
	if err = decode(resp, &env); err != nil {
		return &sendFailure{err.Error(), false}
	}
	if env.Status != "ok" || env.Code == nil || *env.Code != 0 {
		return &sendFailure{fmt.Sprintf("NapCat 未确认发送成功（status=%s）", clip(env.Status, 20)), false}
	}
	if len(env.Data.MessageID) == 0 || string(env.Data.MessageID) == "null" {
		return &sendFailure{"NapCat 成功响应缺少 message_id，请在 QQ 核对后处理", false}
	}
	return nil
}
func (r *Runtime) connect(c Config) (any, error) {
	resp, err := r.request(Request{Method: "POST", Path: c.URL + "/get_login_info", Credential: c.Credential, Headers: map[string]string{"content-type": "application/json"}, Body: body(map[string]any{})})
	if err != nil {
		return nil, errors.New("连接失败，请检查 NapCat 地址、HTTP 服务和托管凭据")
	}
	if resp.Status < 200 || resp.Status >= 300 {
		return nil, oneBotHTTPError(resp.Status)
	}
	var env struct {
		Status string `json:"status"`
		Code   *int   `json:"retcode"`
		Data   struct {
			ID   int64  `json:"user_id"`
			Name string `json:"nickname"`
		} `json:"data"`
	}
	if err = decode(resp, &env); err != nil {
		return nil, err
	}
	if env.Status != "ok" || env.Code == nil || *env.Code != 0 || env.Data.ID <= 0 {
		return nil, errors.New("NapCat 未返回已登录机器人信息")
	}
	return map[string]any{"user_id": strconv.FormatInt(env.Data.ID, 10), "nickname": clip(env.Data.Name, 100)}, nil
}

type Item struct {
	Rating    float64           `json:"community_rating"`
	Genres    []string          `json:"genres"`
	Providers map[string]string `json:"provider_ids"`
	SeriesID  string            `json:"series_id"`
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Type      string            `json:"type"`
	Date      string            `json:"date_created"`
	Series    string            `json:"series_name"`
	Season    int               `json:"parent_index_number"`
	Episode   int               `json:"index_number"`
	Year      int               `json:"production_year"`
}

func parseDate(s string) (time.Time, error) {
	t, e := time.Parse(time.RFC3339Nano, s)
	if e != nil {
		return t, errors.New("Emby 缺少可解析 date_created，已停止推进该游标")
	}
	return t, nil
}
func (r *Runtime) scanEmby(c Config, e *Engine, typ string) error {
	scope := fmt.Sprintf("%d:%s:%s", *c.ProxyID, c.Library, typ)
	cur := e.Cursors[typ]
	if cur.Scope != scope {
		cur = Cursor{Scope: scope}
	}
	var cutoff time.Time
	var err error
	if cur.Ready {
		cutoff, err = parseDate(cur.At)
		if err != nil {
			return err
		}
	}
	var items []Item
	complete := false
	offset := 0
	for page := 0; page < 20; page++ {
		query := url.Values{"type": {typ}, "limit": {"50"}, "offset": {strconv.Itoa(offset)}, "sort_by": {"date_created"}, "sort_order": {"desc"}}
		if *c.ProxyID > 0 {
			query.Set("proxy_id", strconv.Itoa(*c.ProxyID))
		}
		if c.Library != "" {
			query.Set("library_id", c.Library)
		}
		resp, err := r.request(Request{Method: "GET", Path: "/api/plugin-host/emby/items?" + query.Encode()})
		if err != nil {
			return err
		}
		var env struct {
			Items  []Item `json:"items"`
			Total  *int   `json:"total"`
			Limit  int    `json:"limit"`
			Offset int    `json:"offset"`
		}
		if err = decode(resp, &env); err != nil {
			return err
		}
		if env.Total == nil || env.Items == nil || env.Offset != offset || env.Limit < 1 || env.Limit > 50 || len(env.Items) > env.Limit {
			return errors.New("Emby 分页响应不符合公开契约")
		}
		if !cur.Ready && page == 0 && len(env.Items) > 0 {
			cutoff, err = parseDate(env.Items[0].Date)
			if err != nil {
				return err
			}
		}
		for _, i := range env.Items {
			if i.ID == "" || i.Type != typ {
				return errors.New("Emby 条目缺少标识或类型不匹配")
			}
			at, err := parseDate(i.Date)
			if err != nil {
				return err
			}
			if at.Before(cutoff) {
				complete = true
				continue
			}
			items = append(items, i)
		}
		offset += len(env.Items)
		if offset >= *env.Total {
			complete = true
		}
		if complete {
			break
		}
		if len(env.Items) == 0 {
			return errors.New("Emby 分页为空但未到 total，游标未推进")
		}
	}
	if !complete {
		return errors.New("单次增量超过 1000 条，未推进游标；请缩小媒体库范围后重试")
	}
	if len(items) == 0 {
		if !cur.Ready {
			cur.Ready = true
			cur.At = time.Now().UTC().Format(time.RFC3339Nano)
		}
		e.Cursors[typ] = cur
		return nil
	}
	sort.Slice(items, func(i, j int) bool {
		a, _ := parseDate(items[i].Date)
		b, _ := parseDate(items[j].Date)
		if a.Equal(b) {
			return items[i].ID < items[j].ID
		}
		return a.Before(b)
	})
	newest, _ := parseDate(items[len(items)-1].Date)
	ids := []string{}
	if cur.Ready && newest.Equal(cutoff) {
		ids = append(ids, cur.IDs...)
	}
	for _, i := range items {
		at, _ := parseDate(i.Date)
		known := cur.Ready && at.Equal(cutoff) && has(cur.IDs, i.ID)
		if cur.Ready && !known {
			n := r.mediaNotice(c, i)
			n.ID = hashText(scope + ":" + i.ID)
			n.At = at.UTC().Format(time.RFC3339)
			if err = e.enqueue(c, n); err != nil {
				return err
			}
		}
		if at.Equal(newest) && !has(ids, i.ID) {
			ids = append(ids, i.ID)
		}
	}
	if len(ids) > 1000 {
		return errors.New("同一入库时间超过 1000 条，游标未推进")
	}
	e.Cursors[typ] = Cursor{Scope: scope, At: newest.UTC().Format(time.RFC3339Nano), IDs: ids, Ready: true}
	return nil
}
func hashText(s string) string { b := sha256.Sum256([]byte(s)); return hex.EncodeToString(b[:]) }
func pointer(v any, p string) (any, bool) {
	if p == "" {
		return v, true
	}
	for _, part := range strings.Split(strings.TrimPrefix(p, "/"), "/") {
		part = strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
		switch x := v.(type) {
		case map[string]any:
			var ok bool
			v, ok = x[part]
			if !ok {
				return nil, false
			}
		case []any:
			i, e := strconv.Atoi(part)
			if e != nil || i < 0 || i >= len(x) {
				return nil, false
			}
			v = x[i]
		default:
			return nil, false
		}
	}
	return v, true
}
func scalar(v any) (string, bool) {
	switch x := v.(type) {
	case string:
		return x, true
	case json.Number:
		return x.String(), true
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64), true
	}
	return "", false
}
func (r *Runtime) getJSON(path string) (any, error) {
	resp, err := r.request(Request{Method: "GET", Path: path})
	if err != nil {
		return nil, err
	}
	var v any
	if err = decode(resp, &v); err != nil {
		return nil, err
	}
	return v, nil
}
func (r *Runtime) scanAdapter(c Config, e *Engine, a Adapter) error {
	raw, err := r.getJSON(a.Path)
	if err != nil {
		return err
	}
	value, ok := pointer(raw, a.List)
	list, good := value.([]any)
	if !ok || !good {
		return errors.New(a.Key + " 列表映射不匹配；请诊断真实返回 JSON")
	}
	if len(list) > 1000 {
		return errors.New(a.Key + " 返回超过 1000 条，请配置分页窗口")
	}
	ab, _ := json.Marshal(a)
	scope := hashText(string(ab))
	cur := e.Adapters[a.Key]
	if scope != cur.Scope {
		cur = AdapterCursor{Scope: scope}
	}
	seen := append([]string{}, cur.Seen...)
	for _, row := range list {
		idv, ok := pointer(row, a.ID)
		id, good := scalar(idv)
		if !ok || !good || id == "" {
			return errors.New(a.Key + " 标识字段不匹配")
		}
		sv, ok := pointer(row, a.Status)
		status, good := scalar(sv)
		if !ok || !good {
			return errors.New(a.Key + " 状态字段不匹配")
		}
		fingerprint := hashText(id + ":" + status)
		if has(seen, fingerprint) {
			continue
		}
		level, terminal := a.Levels[status]
		if cur.Ready && terminal {
			tv, ok := pointer(row, a.Title)
			title, good := scalar(tv)
			if !ok || !good {
				return errors.New(a.Key + " 标题字段不匹配")
			}
			detail := ""
			if a.Body != "" {
				bv, ok := pointer(row, a.Body)
				var good bool
				detail, good = scalar(bv)
				if !ok || !good {
					return errors.New(a.Key + " 正文字段不匹配")
				}
			}
			if err = e.enqueue(c, Notice{ID: scope + fingerprint, Level: level, Title: safeDisplay(clip(title, 256)), Body: clip(detail, 1500), At: time.Now().UTC().Format(time.RFC3339)}); err != nil {
				return err
			}
		}
		seen = append(seen, fingerprint)
	}
	if len(seen) > 2000 {
		seen = seen[len(seen)-2000:]
	}
	e.Adapters[a.Key] = AdapterCursor{Scope: scope, Seen: seen, Ready: true}
	return nil
}
func (r *Runtime) saveEngine(e *Engine) error {
	_, etagErr, err := r.readEngine()
	if err != nil {
		return err
	}
	return r.write("engine", etagErr, e)
}
func (r *Runtime) readEngine() (Engine, string, error) {
	e := newEngine()
	etag, _, err := r.read("engine", &e)
	if e.Cursors == nil {
		e.Cursors = map[string]Cursor{}
	}
	if e.Adapters == nil {
		e.Adapters = map[string]AdapterCursor{}
	}
	return e, etag, err
}

type Command struct {
	ID    string `json:"id"`
	Retry bool   `json:"retry"`
}

func (r *Runtime) tick(now time.Time) error {
	c, _, err := r.config()
	if err != nil {
		return err
	}
	if !c.Enabled {
		return nil
	}
	if err = c.Validate(); err != nil {
		return err
	}
	e, _, err := r.readEngine()
	if err != nil {
		return err
	}
	var cmd Command
	_, _, err = r.read("command", &cmd)
	if err != nil {
		return err
	}
	force := cmd.ID != "" && cmd.ID != e.LastCommand
	if force {
		e.LastCommand = cmd.ID
		if cmd.Retry {
			for i := range e.Queue {
				if e.Queue[i].Dead {
					e.Queue[i].Destination = c.Destination
				}
				e.Queue[i].Dead = false
				e.Queue[i].Attempts = 0
				e.Queue[i].Next = now.Unix()
			}
		}
	}
	if force || now.Unix()-e.LastPoll >= int64(c.Interval) {
		e.LastPoll = now.Unix()
		e.LastError = ""
		if c.Emby {
			for _, typ := range c.Types {
				trial := cloneEngine(e)
				if err = r.scanEmby(c, &trial, typ); err != nil {
					e.LastError = err.Error()
					e.log("failed", "Emby 检测失败", err.Error())
				} else {
					e = trial
				}
			}
		}
		for _, a := range c.Adapters {
			trial := cloneEngine(e)
			if err = r.scanAdapter(c, &trial, a); err != nil {
				e.LastError = err.Error()
				e.log("failed", "历史检测失败", err.Error())
			} else {
				e = trial
			}
		}
	}
	if err = r.saveEngine(&e); err != nil {
		return err
	} // 先持久化 outbox 和游标，再发送。
	sent := 0
	for i := 0; i < len(e.Queue) && sent < 5; {
		q := &e.Queue[i]
		if q.Dead || q.Next > now.Unix() {
			i++
			continue
		}
		sent++
		q.Attempts++
		err = r.send(q.Destination, q.Message, q.Image)
		if err == nil {
			e.Delivered++
			e.log("succeeded", q.Title, "NapCat 已确认发送")
			e.Queue = append(e.Queue[:i], e.Queue[i+1:]...)
		} else {
			q.LastError = err.Error()
			f, ok := err.(*sendFailure)
			retry := ok && f.retry
			q.Dead = !retry || q.Attempts >= c.MaxAttempts
			delay := int64(10) << uint(q.Attempts-1)
			q.Next = now.Unix() + delay
			e.log("failed", q.Title, q.LastError)
			i++
		}
		if err = r.saveEngine(&e); err != nil {
			return err
		} // 成功发送后若持久化失败，重启可能重复发送。
	}
	return nil
}
func cloneEngine(e Engine) Engine {
	b, _ := json.Marshal(e)
	var copy Engine
	_ = json.Unmarshal(b, &copy)
	return copy
}
func result(status, message string) map[string]any {
	return map[string]any{"status": status, "message": message}
}
func (r *Runtime) Handle(req RPC) (any, error) {
	switch req.Method {
	case "runtime.initialize":
		var p struct {
			Protocol string `json:"protocol"`
		}
		if json.Unmarshal(req.Params, &p) != nil || p.Protocol != "dian115:wasm@1" {
			return nil, errors.New("unsupported protocol")
		}
		return map[string]any{"ready": true, "protocol": p.Protocol}, nil
	case "runtime.shutdown":
		return map[string]any{"stopping": true}, nil
	case "runtime.invoke":
		var p struct {
			Envelope struct {
				Op      string          `json:"op"`
				ID      string          `json:"invocation_id"`
				Payload json.RawMessage `json:"payload"`
			} `json:"envelope"`
		}
		if json.Unmarshal(req.Params, &p) != nil || p.Envelope.ID == "" {
			return nil, errors.New("invalid invocation")
		}
		switch p.Envelope.Op {
		case "state":
			return r.state(p.Envelope.Payload)
		case "action":
			return r.action(p.Envelope.ID, p.Envelope.Payload)
		case "resident":
			for {
				if err := r.tick(time.Now()); err != nil {
					_ = r.host.Call("host.log", map[string]any{"level": "warning", "message": "QQ 通知后台处理失败", "fields": map[string]any{"reason": clip(err.Error(), 512)}}, nil)
				}
				time.Sleep(10 * time.Second)
			}
		default:
			return nil, errors.New("unsupported op")
		}
	default:
		return nil, errors.New("method not found")
	}
}
func (r *Runtime) state(raw json.RawMessage) (any, error) {
	c, _, err := r.config()
	if err != nil {
		return nil, err
	}
	e, _, err := r.readEngine()
	if err != nil {
		return nil, err
	}
	dead := 0
	for _, q := range e.Queue {
		if q.Dead {
			dead++
		}
	}
	displayConfig := c
	displayConfig.Adapters = append([]Adapter{}, c.Adapters...)
	for i := range displayConfig.Adapters {
		a := &displayConfig.Adapters[i]
		a.Path = strings.TrimPrefix(a.Path, "/")
		a.List = strings.TrimPrefix(a.List, "/")
		a.ID = strings.TrimPrefix(a.ID, "/")
		a.Status = strings.TrimPrefix(a.Status, "/")
		a.Title = strings.TrimPrefix(a.Title, "/")
		a.Body = strings.TrimPrefix(a.Body, "/")
	}
	snapshot := map[string]any{"config": displayConfig, "adapter_routes_relative": true, "logs": e.Logs, "pending": len(e.Queue) - dead, "dead": dead, "delivered": e.Delivered, "last_poll": e.LastPoll, "last_error": e.LastError, "baselines": e.Cursors}
	b, _ := json.Marshal(snapshot)
	version := "qq-" + hashText(string(b))[:24]
	etag := `"` + version + `"`
	var p struct {
		ETag string `json:"if_none_match"`
	}
	_ = json.Unmarshal(raw, &p)
	if p.ETag == etag {
		return map[string]any{"not_modified": true, "etag": etag}, nil
	}
	return map[string]any{"state_version": version, "etag": etag, "state": snapshot}, nil
}
func (r *Runtime) action(id string, raw json.RawMessage) (any, error) {
	var p struct {
		ID    string          `json:"id"`
		Input json.RawMessage `json:"input"`
	}
	if json.Unmarshal(raw, &p) != nil || p.ID == "" {
		return nil, errors.New("invalid action")
	}
	c, etag, err := r.config()
	if err != nil {
		return result("failed", err.Error()), nil
	}
	switch p.ID {
	case "save-config":
		next := defaults()
		if err = json.Unmarshal(p.Input, &next); err != nil {
			return result("failed", "配置格式无效"), nil
		}
		if err = next.Validate(); err != nil {
			return result("failed", err.Error()), nil
		}
		if err = r.write("config", etag, next); err != nil {
			return result("failed", err.Error()), nil
		}
		return result("succeeded", "配置已保存；新来源首次检测只建立基线"), nil
	case "test-connection", "send-test":
		if err = c.Validate(); err != nil {
			return result("failed", err.Error()), nil
		}
		if p.ID == "test-connection" {
			bot, err := r.connect(c)
			if err != nil {
				return result("failed", err.Error()), nil
			}
			v := result("succeeded", "NapCat 已登录并可访问")
			v["bot"] = bot
			return v, nil
		}
		receiptKey := "test-" + hashText(id)[:32]
		var previous map[string]any
		receiptETag, exists, receiptErr := r.read(receiptKey, &previous)
		if receiptErr != nil {
			return result("failed", receiptErr.Error()), nil
		}
		if exists {
			if previous["status"] == "accepted" {
				return result("failed", "此测试调用的发送结果未确认，请先在 QQ 核对，避免重复发送"), nil
			}
			return previous, nil
		}
		if err = r.write(receiptKey, receiptETag, result("accepted", "测试发送处理中")); err != nil {
			return result("failed", err.Error()), nil
		}
		n := Notice{Title: "🔔 QQ 通知测试", Body: "DIAN115 → 插件 → NapCat → QQ 通知通道测试。", Level: "info", At: time.Now().UTC().Format(time.RFC3339)}
		outcome := result("succeeded", "NapCat 已确认测试消息，请在 QQ 核对")
		if c.Emby {
			if preview, ok := r.previewNotice(c); ok {
				n = preview
			}
		}
		if err = r.send(c.Destination, render(c, n), n.Image); err != nil {
			outcome = result("failed", err.Error())
		}
		receiptETag, _, receiptErr = r.read(receiptKey, &previous)
		if receiptErr != nil {
			return result("failed", "发送结果持久化未完成，请先在 QQ 核对"), nil
		}
		if err = r.write(receiptKey, receiptETag, outcome); err != nil {
			return result("failed", "发送结果持久化失败，请先在 QQ 核对"), nil
		}
		return outcome, nil
	case "poll-now", "retry-now":
		if !c.Enabled {
			return result("failed", "请先开启自动通知并保存配置"), nil
		}
		var command Command
		ce, _, err := r.read("command", &command)
		if err != nil {
			return result("failed", err.Error()), nil
		}
		if err = r.write("command", ce, Command{ID: id, Retry: p.ID == "retry-now"}); err != nil {
			return result("failed", err.Error()), nil
		}
		return result("accepted", "已提交后台请求，常驻模块约 10 秒内处理"), nil
	case "instances", "libraries":
		path := "/api/plugin-host/emby/instances"
		if p.ID == "libraries" {
			path = "/api/plugin-host/emby/libraries"
			if c.ProxyID != nil && *c.ProxyID > 0 {
				path += "?proxy_id=" + strconv.Itoa(*c.ProxyID)
			}
		}
		v, err := r.getJSON(path)
		if err != nil {
			return result("failed", err.Error()), nil
		}
		res := result("succeeded", "已读取宿主配置")
		res["data"] = v
		return res, nil
	case "diagnose":
		var input struct {
			Path string `json:"path"`
		}
		if json.Unmarshal(p.Input, &input) != nil || !safeHistoryPath(input.Path) {
			return result("failed", "仅允许诊断已声明的历史/任务接口"), nil
		}
		v, err := r.getJSON(input.Path)
		if err != nil {
			return result("failed", err.Error()), nil
		}
		v = redact(v)
		b, _ := json.MarshalIndent(v, "", "  ")
		res := result("succeeded", "响应已脱敏，仅用于配置字段映射；请避免分享私人媒体信息")
		res["sample"] = clip(string(b), 16000)
		return res, nil
	default:
		return result("failed", "未知操作"), nil
	}
}
func redact(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := map[string]any{}
		for k, v := range x {
			l := strings.ToLower(k)
			if strings.Contains(l, "token") || strings.Contains(l, "secret") || strings.Contains(l, "cookie") || strings.Contains(l, "password") || strings.Contains(l, "authorization") || strings.Contains(l, "api_key") || strings.Contains(l, "url") || strings.Contains(l, "path") {
				out[k] = "[已隐藏]"
			} else {
				out[k] = redact(v)
			}
		}
		return out
	case []any:
		if len(x) > 5 {
			x = x[:5]
		}
		for i := range x {
			x[i] = redact(x[i])
		}
		return x
	case string:
		if safeDisplay(x) != x {
			return "[路径已隐藏]"
		}
		if strings.Contains(x, "http://") || strings.Contains(x, "https://") {
			return "[包含链接的文本已隐藏]"
		}
		if !utf8.ValidString(x) {
			return "[无效文本]"
		}
		return clip(x, 512)
	}
	return v
}

// 不将宿主返回的原始绝对文件路径展示到运行时 state。
func safeDisplay(s string) string {
	if strings.HasPrefix(s, "/") || strings.HasPrefix(s, `\\`) || len(s) > 2 && s[1] == ':' && (s[2] == '\\' || s[2] == '/') {
		return "[路径已隐藏]"
	}
	return s
}
