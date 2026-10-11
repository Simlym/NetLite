package dnstask

import "testing"

func sampleTask() Task {
	return Task{ID: "photos", Provider: "cloudflare", Secret: "dummy-secret", IPv4: Source{Domains: []string{"example.com"}, URL: "https://api4.ipify.org"}, Interval: 300, TTL: "600"}
}

func TestTaskValidation(t *testing.T) {
	if err := Validate(sampleTask()); err != nil {
		t.Fatal(err)
	}
	for _, edit := range []func(*Task){
		func(t *Task) { t.ID = "../outside" },
		func(t *Task) { t.Secret = "" },
		func(t *Task) { t.Provider = "unknown" },
		func(t *Task) { t.Interval = 1 },
		func(t *Task) { t.TTL = "-1" },
		func(t *Task) { t.IPv4.URL = "file:///etc/passwd" },
		func(t *Task) { t.IPv4.URL = "https://user:password@example.com" },
		func(t *Task) { t.IPv4.Domains = []string{"bad.example.com?proxied=true"} },
		func(t *Task) { t.IPv4.Domains = []string{"example.com", "EXAMPLE.COM"} },
		func(t *Task) { t.IPv4.Domains = nil },
	} {
		task := sampleTask()
		edit(&task)
		if Validate(task) == nil {
			t.Fatalf("accepted invalid task: provider=%s", task.Provider)
		}
	}
}
