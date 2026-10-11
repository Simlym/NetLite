package ddnsgo

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestStructuredEvents(t *testing.T) {
	c := &child{}
	w := &eventWriter{child: c}
	at := time.Date(2026, 10, 11, 1, 2, 3, 0, time.UTC)
	send := func(e bridgeEvent) {
		t.Helper()
		e.Schema = 1
		e.At = at
		b, _ := json.Marshal(e)
		line := eventPrefix + string(b) + "\r\n"
		for _, p := range []string{line[:9], line[9:]} {
			w.Write([]byte(p))
		}
	}
	w.Write([]byte("Token=secret raw provider failure\n"))
	send(bridgeEvent{Event: "check_started"})
	send(bridgeEvent{Event: "ip", IPv4: "192.0.2.123", IPv6: "2001:db8::1"})
	send(bridgeEvent{Event: "provider_request", Action: "query", ProviderRequests: 1})
	if c.state.UpdateRequests != 0 || c.state.ResultKnown {
		t.Fatal("query falsely reported as successful update")
	}
	send(bridgeEvent{Event: "provider_request", Action: "update", ProviderRequests: 2, UpdateRequests: 1})
	send(bridgeEvent{Event: "provider_response", HTTPStatus: 200})
	send(bridgeEvent{Event: "check_finished", IPv4: "192.0.2.123", IPv6: "2001:db8::1", IPv4Result: "success", IPv6Result: "unchanged", ProviderRequests: 2, UpdateRequests: 1})
	s := c.state
	if !s.ResultKnown || s.Checking || s.IPv4 != "192.0.2.123" || s.IPv6 != "2001:db8::1" || s.UpdateRequests != 1 || s.ProviderRequests != 2 || s.LastFinished != at || s.LastRequest != at {
		t.Fatalf("unexpected state: %+v", s)
	}
	if strings.Contains(s.Message, "secret") {
		t.Fatal("raw log leaked")
	}
	send(bridgeEvent{Event: "check_started"})
	if c.state.ResultKnown || c.state.IPv4Result != "" || c.state.UpdateRequests != 0 {
		t.Fatal("new check retained previous success")
	}
	send(bridgeEvent{Event: "check_finished", IPv4Result: "ip_failed", IPv6Result: "disabled"})
	if c.state.UpdateRequests != 0 || c.state.Message != "公网 IP 获取失败" {
		t.Fatal("IP failure misrepresented as update")
	}
}

func TestEventInputBoundary(t *testing.T) {
	c := &child{}
	w := &eventWriter{child: c}
	for _, line := range []string{eventPrefix + `{"schema":2,"event":"check_started","at":"2026-10-11T00:00:00Z"}`, eventPrefix + `{"schema":1,"event":"check_started"}`, eventPrefix + `not json`, eventPrefix + strings.Repeat("x", 17000)} {
		w.Write([]byte(line + "\n"))
	}
	if c.state.Checking {
		t.Fatal("invalid events accepted")
	}
	w.Write([]byte(eventPrefix + `{"schema":1,"event":"ip","at":"2026-10-11T00:00:00Z","ipv4":"token=secret","ipv6":"192.0.2.1"}` + "\n"))
	if c.state.IPv4 != "" || c.state.IPv6 != "" {
		t.Fatal("invalid or wrong-family IP accepted")
	}
	w.Write([]byte(eventPrefix + `{"schema":1,"event":"provider_request","at":"2026-10-11T00:00:00Z","provider_requests":1,"update_requests":2}` + "\n"))
	if c.state.ProviderRequests != 0 {
		t.Fatal("inconsistent request counters accepted")
	}
}
