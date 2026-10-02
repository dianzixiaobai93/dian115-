package main

import (
	"strings"
	"testing"
)

type checkHost struct {
	calls   []Request
	invalid bool
}

func (h *checkHost) Call(method string, params, target any) error {
	q := params.(Request)
	h.calls = append(h.calls, q)
	v := map[string]any{"errcode": 0, "access_token": "temporary-token", "expires_in": 7200}
	if strings.Contains(q.Path, "message/send") {
		v = map[string]any{"errcode": 0}
		if h.invalid {
			v["invaliduser"] = "missing"
		}
	}
	*(target.(*Response)) = Response{Status: 200, Body: body(v)}
	return nil
}
func TestWecomDelivery(t *testing.T) {
	d := Destination{URL: "https://qyapi.weixin.qq.com", Credential: "cred_test", CorpID: "ww_test", AgentID: 1, Kind: "private", ID: "member"}
	h := &checkHost{}
	r := Runtime{host: h}
	if e := r.send(d, "测试\n内容", "https://image.tmdb.org/t/p/w780/sample.jpg"); e != nil {
		t.Fatal(e)
	}
	if len(h.calls) != 2 || h.calls[0].Credential != "cred_test" || strings.Contains(h.calls[0].Path, "corpsecret") {
		t.Fatal("Secret must be injected by host")
	}
	var payload map[string]any
	decode(Response{Status: 200, Body: h.calls[1].Body}, &payload)
	if payload["msgtype"] != "news" || payload["touser"] != "member" {
		t.Fatal(payload)
	}
	h.invalid = true
	if r.send(d, "test") == nil {
		t.Fatal("invalid recipient treated as success")
	}
	if len(h.calls) != 3 {
		t.Fatal("token was not cached")
	}
	d.ID = "@all"
	if validateDestination(d) == nil {
		t.Fatal("broadcast accepted")
	}
	if got := byteClip("中文", 4); got != "中" {
		t.Fatal(got)
	}
}
