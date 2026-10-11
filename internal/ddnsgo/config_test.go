package ddnsgo

import (
	"encoding/json"
	"strings"
	"testing"
)

func sampleTask() Task {
	return Task{ID: "photos", Provider: "cloudflare", Secret: "dummy-test-secret", IPv4: Source{Domains: []string{"example.com", "*.example.com"}, URL: "https://api4.ipify.org"}, Interval: 300}
}
func TestConfigMapping(t *testing.T) {
	tsk := sampleTask()
	tsk.IPv6 = Source{Domains: []string{"v6.example.com"}, URL: "https://api6.ipify.org"}
	b, e := Render(tsk)
	if e != nil {
		t.Fatal(e)
	}
	var c struct {
		Tasks []taskConfig `json:"dnsconf"`
		NoWAN bool         `json:"notallowwanaccess"`
	}
	if e = json.Unmarshal(b, &c); e != nil {
		t.Fatal(e)
	}
	if len(c.Tasks) != 1 || !c.NoWAN || c.Tasks[0].DNS.Secret != tsk.Secret || !c.Tasks[0].IPv6.Enable || c.Tasks[0].IPv4.Domains[1] != "*.example.com" {
		t.Fatal("configuration mapping lost fields")
	}
}
func TestRejectUnsafeInput(t *testing.T) {
	for _, change := range []func(*Task){func(t *Task) { t.ID = "../outside" }, func(t *Task) { t.Provider = "unknown" }, func(t *Task) { t.Secret = "" }, func(t *Task) { t.IPv4.URL = "file:///etc/passwd" }, func(t *Task) { t.IPv4.Domains = []string{"bad.example.com?proxied=true"} }, func(t *Task) { t.Interval = 0 }} {
		tsk := sampleTask()
		change(&tsk)
		_, e := Render(tsk)
		if e == nil {
			t.Fatal("invalid input accepted")
		}
		if strings.Contains(e.Error(), tsk.Secret) && tsk.Secret != "" {
			t.Fatal("error leaked credential")
		}
	}
}
