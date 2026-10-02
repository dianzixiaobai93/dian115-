package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"
)

type mockHost struct {
	idempotency         map[string]string
	tmdb                any
	store               map[string]json.RawMessage
	revisions           map[string]int
	items               []Item
	history             any
	sent                []Request
	sendStatus          int
	sendPayload         any
	failEngineWrite     bool
	requireDurableQueue bool
	calls               int
}

func mock() *mockHost {
	return &mockHost{idempotency: map[string]string{}, store: map[string]json.RawMessage{}, revisions: map[string]int{}, sendStatus: 200, sendPayload: map[string]any{"status": "ok", "retcode": 0, "data": map[string]any{"message_id": 42}}}
}
func response(v any) Response {
	return Response{Status: 200, Headers: map[string][]string{}, Body: body(v)}
}
func copyInto(v, out any) error {
	if out == nil {
		return nil
	}
	b, _ := json.Marshal(v)
	return json.Unmarshal(b, out)
}
func (m *mockHost) Call(method string, params, out any) error {
	m.calls++
	if method == "host.log" {
		return nil
	}
	if method != "host.call" {
		return errors.New("unexpected method")
	}
	req := params.(Request)
	u, _ := url.Parse(req.Path)
	resp := response(map[string]any{})
	if strings.HasPrefix(u.Path, "/api/plugin-runtime/storage/") {
		key := strings.TrimPrefix(u.Path, "/api/plugin-runtime/storage/")
		value, exists := m.store[key]
		etag := fmt.Sprintf(`"pkv_%d"`, m.revisions[key])
		switch req.Method {
		case "GET":
			if !exists {
				resp.Status = 404
			} else {
				resp = response(map[string]any{"data": map[string]any{"value": value, "revision": fmt.Sprintf("pkv_%d", m.revisions[key])}, "meta": map[string]any{}})
				resp.Headers["ETag"] = []string{etag}
			}
		case "PUT":
			id := req.Headers["idempotency-key"]
			fingerprint := req.Path + ":" + req.Headers["if-match"] + ":" + req.Body
			if old, exists := m.idempotency[id]; exists && old != fingerprint {
				resp.Status = 409
				break
			}
			m.idempotency[id] = fingerprint
			if key == "engine" && m.failEngineWrite {
				resp.Status = 500
				break
			}
			if req.Headers["if-match"] != "" && req.Headers["if-match"] != etag {
				resp.Status = 412
				break
			}
			data, _ := base64.RawStdEncoding.DecodeString(req.Body)
			var envelope struct {
				Value json.RawMessage `json:"value"`
			}
			_ = json.Unmarshal(data, &envelope)
			m.store[key] = envelope.Value
			m.revisions[key]++
		default:
			return errors.New("unexpected storage method")
		}
	} else if u.Path == "/api/plugin-host/emby/items" {
		offset := 0
		_, _ = fmt.Sscanf(u.Query().Get("offset"), "%d", &offset)
		end := offset + 50
		if end > len(m.items) {
			end = len(m.items)
		}
		if offset > end {
			offset = end
		}
		page := append([]Item{}, m.items[offset:end]...)
		resp = response(map[string]any{"items": page, "total": len(m.items), "offset": offset, "limit": 50})
	} else if strings.HasPrefix(u.Path, "/api/tmdb/") {
		resp = response(m.tmdb)
	} else if safeHistoryPath(req.Path) {
		resp = response(m.history)
	} else if strings.HasPrefix(u.Path, "/send_") {
		if m.requireDurableQueue {
			var e Engine
			_ = json.Unmarshal(m.store["engine"], &e)
			if len(e.Queue) == 0 {
				return errors.New("send before outbox persisted")
			}
		}
		m.sent = append(m.sent, req)
		resp = response(m.sendPayload)
		resp.Status = m.sendStatus
	} else if u.Path == "/get_login_info" {
		resp = response(map[string]any{"status": "ok", "retcode": 0, "data": map[string]any{"user_id": 12345, "nickname": "测试机器人"}})
	} else {
		return errors.New("unexpected route " + req.Path)
	}
	return copyInto(resp, out)
}
func configForTest() Config {
	c := defaults()
	c.ID = "123456789"
	c.Credential = "cred_test"
	c.Enabled = true
	c.Emby = true
	p := 1
	c.ProxyID = &p
	c.Types = []string{"Movie"}
	return c
}
func saveConfig(m *mockHost, c Config) {
	b, _ := json.Marshal(c)
	m.store["config"] = b
	m.revisions["config"] = 1
}
func item(id, date string) Item { return Item{ID: id, Name: "电影 " + id, Type: "Movie", Date: date} }
func action(r *Runtime, id, kind string, input any) (map[string]any, error) {
	p, _ := json.Marshal(map[string]any{"id": kind, "input": input})
	v, err := r.action(id, p)
	if err != nil {
		return nil, err
	}
	return v.(map[string]any), nil
}

func TestBaselineSkipsOldLibraryAndHandlesTimestampTies(t *testing.T) {
	m := mock()
	r := Runtime{host: m}
	c := configForTest()
	e := newEngine()
	date := "2026-10-01T10:00:00Z"
	for i := 0; i < 125; i++ {
		m.items = append(m.items, item(fmt.Sprint(i), date))
	}
	m.items = append(m.items, item("older", "2026-09-01T10:00:00Z"))
	if err := r.scanEmby(c, &e, "Movie"); err != nil {
		t.Fatal(err)
	}
	if len(e.Queue) != 0 || len(e.Cursors["Movie"].IDs) != 125 {
		t.Fatalf("baseline %d / ids %d", len(e.Queue), len(e.Cursors["Movie"].IDs))
	}
	m.items = append([]Item{item("new", "2026-10-01T10:01:00Z"), item("tie", date)}, m.items...)
	if err := r.scanEmby(c, &e, "Movie"); err != nil {
		t.Fatal(err)
	}
	if len(e.Queue) != 2 {
		t.Fatalf("want 2 incremental notices, got %d", len(e.Queue))
	}
	if err := r.scanEmby(c, &e, "Movie"); err != nil {
		t.Fatal(err)
	}
	if len(e.Queue) != 2 {
		t.Fatal("duplicate notices after second poll")
	}
}
func TestDurableOutboxRetryAndRestart(t *testing.T) {
	m := mock()
	m.requireDurableQueue = true
	r := Runtime{host: m}
	c := configForTest()
	saveConfig(m, c)
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	m.items = []Item{item("old", "2026-09-01T10:00:00Z")}
	if err := r.tick(now); err != nil {
		t.Fatal(err)
	}
	if len(m.sent) != 0 {
		t.Fatal("baseline sent")
	}
	m.items = append([]Item{item("new", "2026-10-01T11:00:00Z")}, m.items...)
	m.sendStatus = 502
	if err := r.tick(now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	e, _, _ := r.readEngine()
	if len(e.Queue) != 1 || e.Queue[0].Attempts != 1 || e.Queue[0].Dead {
		t.Fatalf("retry not saved: %+v", e.Queue)
	}
	restarted := Runtime{host: m}
	m.sendStatus = 200
	if err := restarted.tick(now.Add(75 * time.Second)); err != nil {
		t.Fatal(err)
	}
	e, _, _ = restarted.readEngine()
	if len(e.Queue) != 0 || e.Delivered != 1 {
		t.Fatal("retry did not succeed")
	}
	if err := restarted.tick(now.Add(2 * time.Minute)); err != nil {
		t.Fatal(err)
	}
	if len(m.sent) != 2 {
		t.Fatalf("duplicate after restart: %d", len(m.sent))
	}
	req := m.sent[1]
	if req.Credential != "cred_test" || req.Headers["authorization"] != "" {
		t.Fatal("credential not delegated")
	}
	b, _ := base64.RawStdEncoding.DecodeString(req.Body)
	var payload map[string]any
	_ = json.Unmarshal(b, &payload)
	if _, ok := payload["message"].([]any); !ok {
		t.Fatal("message must be literal OneBot text segments")
	}
}
func TestStorageFailurePreventsSending(t *testing.T) {
	m := mock()
	r := Runtime{host: m}
	c := configForTest()
	saveConfig(m, c)
	now := time.Now()
	m.items = []Item{item("old", "2026-09-01T00:00:00Z")}
	if err := r.tick(now); err != nil {
		t.Fatal(err)
	}
	m.items = append([]Item{item("new", "2026-10-01T00:00:00Z")}, m.items...)
	m.failEngineWrite = true
	if err := r.tick(now.Add(time.Minute)); err == nil {
		t.Fatal("storage failure ignored")
	}
	if len(m.sent) > 0 {
		t.Fatal("sent without durable queue")
	}
	m.failEngineWrite = false
	if err := r.tick(now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if len(m.sent) != 1 {
		t.Fatal("retry lost event")
	}
}
func TestMalformedEmbyPageDoesNotAdvanceCursor(t *testing.T) {
	m := mock()
	r := Runtime{host: m}
	c := configForTest()
	e := newEngine()
	m.items = []Item{item("old", "2026-09-01T00:00:00Z")}
	if err := r.scanEmby(c, &e, "Movie"); err != nil {
		t.Fatal(err)
	}
	before := e.Cursors["Movie"]
	m.items = append([]Item{item("new", "")}, m.items...)
	trial := cloneEngine(e)
	if err := r.scanEmby(c, &trial, "Movie"); err == nil {
		t.Fatal("invalid date accepted")
	}
	if e.Cursors["Movie"].At != before.At || len(e.Queue) != 0 {
		t.Fatal("cursor advanced on error")
	}
}
func TestAdapterRequiresExplicitShapeAndTracksTransitions(t *testing.T) {
	m := mock()
	r := Runtime{host: m}
	c := configForTest()
	e := newEngine()
	a := Adapter{Key: "organize", Path: "/api/organize/history", List: "/data/items", ID: "/id", Status: "/status", Title: "/title", Body: "/detail", Levels: map[string]string{"done": "success", "error": "failed"}}
	m.history = map[string]any{"data": map[string]any{"items": []any{map[string]any{"id": "a", "status": "running", "title": "任务 A", "detail": "进行中"}}}}
	if err := r.scanAdapter(c, &e, a); err != nil {
		t.Fatal(err)
	}
	if len(e.Queue) != 0 {
		t.Fatal("adapter baseline sent")
	}
	m.history = map[string]any{"data": map[string]any{"items": []any{map[string]any{"id": "a", "status": "done", "title": "任务 A", "detail": "完成"}}}}
	if err := r.scanAdapter(c, &e, a); err != nil {
		t.Fatal(err)
	}
	if len(e.Queue) != 1 {
		t.Fatal("terminal transition missed")
	}
	if err := r.scanAdapter(c, &e, a); err != nil {
		t.Fatal(err)
	}
	if len(e.Queue) != 1 {
		t.Fatal("adapter repeated event")
	}
	m.history = map[string]any{"wrong": []any{}}
	if err := r.scanAdapter(c, &e, a); err == nil {
		t.Fatal("guessed response shape")
	}
}
func TestOneBotBusinessFailuresAndGroupTarget(t *testing.T) {
	m := mock()
	r := Runtime{host: m}
	d := configForTest().Destination
	d.Kind = "group"
	d.ID = "987654321"
	m.sendPayload = map[string]any{"status": "failed", "retcode": 1200}
	if err := r.send(d, "测试 [CQ:at,qq=all]"); err == nil {
		t.Fatal("business failure accepted")
	}
	if !strings.HasSuffix(m.sent[0].Path, "/send_group_msg") {
		t.Fatal("wrong target action")
	}
	b, _ := base64.RawStdEncoding.DecodeString(m.sent[0].Body)
	if !strings.Contains(string(b), `"group_id":987654321`) || !strings.Contains(string(b), `"type":"text"`) {
		t.Fatal(string(b))
	}
	m.sendPayload = map[string]any{"status": "ok", "retcode": 0}
	if err := r.send(d, "x"); err == nil {
		t.Fatal("missing receipt accepted")
	}
}
func TestPermanentFailuresStayDeadAndDisabledDoesNotSend(t *testing.T) {
	m := mock()
	r := Runtime{host: m}
	c := configForTest()
	saveConfig(m, c)
	e := newEngine()
	_ = e.enqueue(c, Notice{ID: "n", Level: "success", Title: "测试", At: time.Now().UTC().Format(time.RFC3339)})
	if err := r.saveEngine(&e); err != nil {
		t.Fatal(err)
	}
	m.sendStatus = 401
	if err := r.tick(time.Now()); err != nil {
		t.Fatal(err)
	}
	e, _, _ = r.readEngine()
	if len(e.Queue) != 1 || !e.Queue[0].Dead {
		t.Fatal("401 should require intervention")
	}
	if err := r.tick(time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if len(m.sent) != 1 {
		t.Fatal("retrying permanent error")
	}
	c.Enabled = false
	saveConfig(m, c)
	if err := r.tick(time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if len(m.sent) != 1 {
		t.Fatal("sent while disabled")
	}
}
func TestReplayTestInvocationDoesNotSendTwice(t *testing.T) {
	m := mock()
	r := Runtime{host: m}
	c := configForTest()
	saveConfig(m, c)
	a, err := action(&r, "same-invocation", "send-test", map[string]any{})
	if err != nil || a["status"] != "succeeded" {
		t.Fatal(a, err)
	}
	a, err = action(&r, "same-invocation", "send-test", map[string]any{})
	if err != nil || a["status"] != "succeeded" || len(m.sent) != 1 {
		t.Fatal("replay sent twice", a, err)
	}
}
func TestConfigValidationAndRedaction(t *testing.T) {
	c := configForTest()
	for _, bad := range []string{"ftp://x", "http://user:password@x", "http://x/?token=a", "http://x/a", "HTTP://x"} {
		copy := c
		copy.URL = bad
		if copy.Validate() == nil {
			t.Fatalf("accepted %s", bad)
		}
	}
	c.Types = []string{"Wrong"}
	if c.Validate() == nil {
		t.Fatal("accepted wrong type")
	}
	v := redact(map[string]any{"password": "x", "api_key": "x", "source": "/vol1/secret", "name": "movie", "url": "http://x/?token=a"}).(map[string]any)
	b, _ := json.Marshal(v)
	if strings.Contains(string(b), "/vol1") || strings.Contains(string(b), "token=a") {
		t.Fatal("diagnostic leaked credentials/path")
	}
}

func TestExplicitRetryRebindsDeadQueueToCurrentDestination(t *testing.T) {
	m := mock()
	r := Runtime{host: m}
	c := configForTest()
	saveConfig(m, c)
	e := newEngine()
	_ = e.enqueue(c, Notice{ID: "dead", Level: "success", Title: "重试"})
	e.Queue[0].Dead = true
	e.Queue[0].Attempts = 3
	if err := r.saveEngine(&e); err != nil {
		t.Fatal(err)
	}
	c.Kind = "group"
	c.ID = "222333444"
	c.Credential = "cred_new"
	saveConfig(m, c)
	if err := r.write("command", "", Command{ID: "explicit-retry", Retry: true}); err != nil {
		t.Fatal(err)
	}
	if err := r.tick(time.Now()); err != nil {
		t.Fatal(err)
	}
	if len(m.sent) != 1 || !strings.HasSuffix(m.sent[0].Path, "/send_group_msg") || m.sent[0].Credential != "cred_new" {
		t.Fatal("dead queue was not explicitly rebound")
	}
	b, _ := base64.RawStdEncoding.DecodeString(m.sent[0].Body)
	if !strings.Contains(string(b), `"group_id":222333444`) {
		t.Fatal(string(b))
	}
}
