package dnstask

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestProvidersRecordLifecycle(t *testing.T) {
	for _, provider := range []string{"cloudflare", "dnspod", "tencentcloud", "alidns"} {
		for _, mode := range []string{"create", "update", "unchanged", "ambiguous", "mismatch", "rejected"} {
			t.Run(provider+"/"+mode, func(t *testing.T) {
				writes := 0
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					respond := func(v any) { json.NewEncoder(w).Encode(v) }
					value := "192.0.2.1"
					if mode == "unchanged" {
						value = "192.0.2.2"
					}
					name := "example.com"
					sub := "@"
					if mode == "mismatch" {
						name, sub = "other.example.com", "other"
					}
					switch provider {
					case "cloudflare":
						if r.Header.Get("Authorization") != "Bearer dummy-secret" {
							t.Error("missing Cloudflare authentication")
						}
						if r.URL.Path == "/zones" {
							respond(map[string]any{"success": true, "result": []any{map[string]string{"id": "zone", "name": "example.com", "status": "active"}}})
							return
						}
						if r.Method == "GET" {
							if r.URL.Query().Get("name") != "example.com" || r.URL.Query().Get("type") != "A" {
								t.Error("unfiltered record lookup")
							}
							record := map[string]any{"id": "record", "name": name, "type": "A", "content": value, "ttl": 120, "proxied": true}
							records := []any{record}
							if mode == "create" {
								records = []any{}
							}
							if mode == "ambiguous" {
								records = append(records, record)
							}
							respond(map[string]any{"success": true, "result": records, "result_info": map[string]int{"total_count": len(records)}})
							return
						}
						var body map[string]any
						json.NewDecoder(r.Body).Decode(&body)
						if mode == "create" {
							if r.Method != "POST" || body["ttl"] != float64(600) || body["proxied"] != false {
								t.Error("invalid create")
							}
						} else if r.Method != "PATCH" || len(body) != 1 {
							t.Error("update changed Cloudflare attributes")
						}
						if body["content"] != "192.0.2.2" {
							t.Error("incorrect update IP")
						}
						writes++
						respond(map[string]any{"success": mode != "rejected", "result": map[string]string{"id": "record"}})
					case "dnspod":
						r.ParseForm()
						v := r.PostForm
						if v.Get("login_token") != "id,dummy-secret" {
							t.Error("missing DNSPod authentication")
						}
						if r.URL.Path == "/Domain.Info" {
							respond(map[string]any{"status": map[string]string{"code": "1"}, "domain": map[string]string{"name": "example.com"}})
							return
						}
						if r.URL.Path == "/Record.List" {
							record := map[string]string{"id": "record", "name": sub, "type": "A", "value": value, "ttl": "120", "line": "电信", "enabled": "1"}
							records := []any{record}
							if mode == "create" {
								records = []any{}
							}
							if mode == "ambiguous" {
								records = append(records, record)
							}
							respond(map[string]any{"status": map[string]string{"code": "1"}, "records": records})
							return
						}
						if mode == "create" {
							if r.URL.Path != "/Record.Create" || v.Get("ttl") != "600" {
								t.Error("invalid DNSPod create")
							}
						} else if r.URL.Path != "/Record.Modify" || v.Get("ttl") != "120" || v.Get("record_line") != "电信" || v.Get("record_id") != "record" {
							t.Error("DNSPod attributes lost")
						}
						if v.Get("value") != "192.0.2.2" {
							t.Error("incorrect update IP")
						}
						writes++
						code := "1"
						if mode == "rejected" {
							code = "7"
						}
						respond(map[string]any{"status": map[string]string{"code": code, "message": "secret=dummy-secret"}})
					case "tencentcloud":
						var body map[string]any
						json.NewDecoder(r.Body).Decode(&body)
						action := r.Header.Get("X-TC-Action")
						if !strings.HasPrefix(r.Header.Get("Authorization"), "TC3-HMAC-SHA256 Credential=id/") || r.Header.Get("X-TC-Version") != "2021-03-23" {
							t.Error("missing Tencent authentication")
						}
						if action == "DescribeDomain" {
							respond(map[string]any{"Response": map[string]any{"RequestId": "request", "DomainInfo": map[string]string{"Domain": "example.com"}}})
							return
						}
						if action == "DescribeRecordList" {
							record := map[string]any{"RecordId": 123, "Name": sub, "Type": "A", "Value": value, "TTL": 120, "Line": "电信", "LineId": "10=0", "Status": "ENABLE"}
							records := []any{record}
							if mode == "create" {
								records = []any{}
							}
							if mode == "ambiguous" {
								records = append(records, record)
							}
							respond(map[string]any{"Response": map[string]any{"RequestId": "request", "RecordList": records, "RecordCountInfo": map[string]int{"TotalCount": len(records)}}})
							return
						}
						if mode == "create" {
							if action != "CreateRecord" || body["TTL"] != float64(600) {
								t.Error("invalid Tencent create")
							}
						} else if action != "ModifyRecord" || body["TTL"] != float64(120) || body["RecordLine"] != "电信" || body["RecordId"] != float64(123) {
							t.Error("Tencent attributes lost")
						}
						if body["Value"] != "192.0.2.2" {
							t.Error("incorrect update IP")
						}
						writes++
						response := map[string]any{"RequestId": "request"}
						if mode == "rejected" {
							response["Error"] = map[string]string{"Code": "AuthFailure", "Message": "secret=dummy-secret"}
						}
						respond(map[string]any{"Response": response})
					case "alidns":
						v := r.URL.Query()
						action := v.Get("Action")
						if v.Get("AccessKeyId") != "id" || v.Get("Signature") == "" || v.Get("SignatureNonce") == "" {
							t.Error("missing AliDNS authentication")
						}
						if action == "DescribeDomainInfo" {
							respond(map[string]string{"RequestId": "request", "DomainName": "example.com"})
							return
						}
						if action == "DescribeSubDomainRecords" {
							record := map[string]any{"RecordId": "record", "DomainName": name, "RR": sub, "Type": "A", "Value": value, "TTL": 120, "Line": "telecom", "Status": "ENABLE"}
							records := []any{record}
							if mode == "create" {
								records = []any{}
							}
							if mode == "ambiguous" {
								records = append(records, record)
							}
							respond(map[string]any{"RequestId": "request", "TotalCount": len(records), "DomainRecords": map[string]any{"Record": records}})
							return
						}
						if mode == "create" {
							if action != "AddDomainRecord" || v.Get("TTL") != "600" {
								t.Error("invalid AliDNS create")
							}
						} else if action != "UpdateDomainRecord" || v.Get("TTL") != "120" || v.Get("Line") != "telecom" || v.Get("RecordId") != "record" {
							t.Error("AliDNS attributes lost")
						}
						if v.Get("Value") != "192.0.2.2" {
							t.Error("incorrect update IP")
						}
						writes++
						if mode == "rejected" {
							w.WriteHeader(400)
							respond(map[string]string{"Code": "Forbidden", "Message": "secret=dummy-secret"})
							return
						}
						respond(map[string]string{"RequestId": "request", "RecordId": "record"})
					}
				}))
				defer server.Close()
				task := sampleTask()
				task.Provider, task.CredentialID = provider, "id"
				state := State{}
				p := providerAPI{task: task, state: &state, client: server.Client(), endpoints: endpoints{server.URL, server.URL, server.URL, server.URL}, zones: map[string]string{}}
				changed, err := p.sync(context.Background(), "example.com", "A", "192.0.2.2")
				wantError := mode == "ambiguous" || mode == "mismatch" || mode == "rejected"
				if (err != nil) != wantError {
					t.Fatalf("unexpected error: %v", err)
				}
				if err != nil && strings.Contains(err.Error(), task.Secret) {
					t.Fatal("credential leaked")
				}
				wantChanged := mode == "create" || mode == "update"
				if changed != wantChanged {
					t.Fatalf("changed=%v", changed)
				}
				wantWrites := 0
				if wantChanged || mode == "rejected" {
					wantWrites = 1
				}
				if writes != wantWrites || state.UpdateRequests != wantWrites {
					t.Fatalf("writes=%d, counter=%d", writes, state.UpdateRequests)
				}
			})
		}
	}
}

func TestCloudflareDelegatedZoneAndWildcard(t *testing.T) {
	var lookedUp []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/zones" {
			name := r.URL.Query().Get("name")
			lookedUp = append(lookedUp, name)
			zones := []any{}
			if name == "sub.example.co.uk" {
				zones = append(zones, map[string]string{"id": "delegated", "name": name, "status": "active"})
			}
			json.NewEncoder(w).Encode(map[string]any{"success": true, "result": zones})
			return
		}
		if r.Method == "GET" {
			json.NewEncoder(w).Encode(map[string]any{"success": true, "result": []any{}})
			return
		}
		if r.URL.Path != "/zones/delegated/dns_records" {
			t.Error("wrong delegated zone")
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["name"] != "*.sub.example.co.uk" {
			t.Error("wildcard changed")
		}
		fmt.Fprint(w, `{"success":true,"result":{"id":"new"}}`)
	}))
	defer server.Close()
	state := State{}
	p := providerAPI{task: sampleTask(), client: server.Client(), state: &state, endpoints: endpoints{Cloudflare: server.URL}, zones: map[string]string{}}
	if changed, err := p.sync(context.Background(), "*.sub.example.co.uk", "A", "192.0.2.2"); err != nil || !changed {
		t.Fatalf("sync: %v", err)
	}
	if len(lookedUp) != 1 || lookedUp[0] != "sub.example.co.uk" {
		t.Fatal("incorrect zone candidates")
	}
}

func TestAliDNSMissingSubzoneHTTP400(t *testing.T) {
	var lookedUp []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("DomainName")
		lookedUp = append(lookedUp, name)
		if name != "example.co.uk" {
			w.WriteHeader(400)
			fmt.Fprint(w, `{"Code":"InvalidDomainName.NoExist"}`)
			return
		}
		fmt.Fprint(w, `{"RequestId":"request","DomainName":"example.co.uk"}`)
	}))
	defer server.Close()
	state := State{}
	task := sampleTask()
	task.CredentialID = "id"
	p := providerAPI{task: task, client: server.Client(), state: &state, endpoints: endpoints{AliDNS: server.URL}, zones: map[string]string{}}
	zone, err := p.aliZone(context.Background(), "home.example.co.uk")
	if err != nil || zone != "example.co.uk" || len(lookedUp) != 2 {
		t.Fatalf("zone=%s, err=%v", zone, err)
	}
}

func TestMalformedLookupCannotCreateRecord(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{"success":true,"result":null}`) }))
	defer server.Close()
	state := State{}
	p := providerAPI{task: sampleTask(), client: server.Client(), state: &state, endpoints: endpoints{Cloudflare: server.URL}, zones: map[string]string{"example.com": "zone"}}
	if _, err := p.sync(context.Background(), "example.com", "A", "192.0.2.2"); err == nil || state.UpdateRequests != 0 {
		t.Fatal("malformed lookup caused write")
	}
}

func TestNoRecordsCodesAreNotAuthenticationErrors(t *testing.T) {
	for _, code := range []string{"10", "7"} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprintf(w, `{"status":{"code":%q}}`, code) }))
		state := State{}
		p := providerAPI{task: sampleTask(), client: server.Client(), state: &state, endpoints: endpoints{DNSPod: server.URL}, zones: map[string]string{}}
		err := p.podCall(context.Background(), "Record.List", url.Values{}, false, nil)
		server.Close()
		if errorCode(err) != code {
			t.Fatal("lost API error classification")
		}
	}
}
