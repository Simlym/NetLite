package dnsprovider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"netlite/internal/model"
	"strings"
	"testing"
)

func TestTXTValueOwnershipAcrossProviders(t *testing.T) {
	for _, provider := range Providers {
		t.Run(provider, func(t *testing.T) {
			records := map[string]string{"1": "unrelated", "2": "existing-value"}
			created, deleted := 0, 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				send := func(v any) { json.NewEncoder(w).Encode(v) }
				switch provider {
				case "cloudflare":
					if r.URL.Path == "/zones" {
						send(map[string]any{"success": true, "result": []any{map[string]string{"id": "zone", "name": "example.com", "status": "active"}}})
						return
					}
					if r.Method == "GET" {
						list := []any{}
						for id, v := range records {
							list = append(list, map[string]string{"id": id, "name": "_acme-challenge.example.com", "type": "TXT", "content": v})
						}
						send(map[string]any{"success": true, "result": list})
						return
					}
					if r.Method == "DELETE" {
						if !strings.HasSuffix(r.URL.Path, "/3") {
							t.Error("deleted unrelated record")
						}
						delete(records, "3")
						deleted++
						send(map[string]any{"success": true, "result": map[string]string{"id": "3"}})
						return
					}
					var body struct{ Content string }
					json.NewDecoder(r.Body).Decode(&body)
					records["3"] = body.Content
					created++
					send(map[string]any{"success": true, "result": map[string]string{"id": "3"}})
				case "dnspod":
					r.ParseForm()
					action := r.URL.Path
					if action == "/Domain.Info" {
						send(map[string]any{"status": map[string]string{"code": "1"}, "domain": map[string]string{"name": "example.com"}})
						return
					}
					if action == "/Record.List" {
						list := []any{}
						for id, v := range records {
							list = append(list, map[string]string{"id": id, "name": "_acme-challenge", "type": "TXT", "value": v})
						}
						send(map[string]any{"status": map[string]string{"code": "1"}, "records": list})
						return
					}
					if action == "/Record.Remove" {
						if r.Form.Get("record_id") != "3" {
							t.Error("deleted unrelated record")
						}
						delete(records, "3")
						deleted++
					} else if action == "/Record.Create" {
						records["3"] = r.Form.Get("value")
						created++
					} else {
						t.Errorf("unexpected action %s", action)
					}
					send(map[string]any{"status": map[string]string{"code": "1"}, "record": map[string]string{"id": "3"}})
				case "tencentcloud":
					action := r.Header.Get("X-TC-Action")
					var body map[string]any
					json.NewDecoder(r.Body).Decode(&body)
					response := map[string]any{"RequestId": "request"}
					if action == "DescribeDomain" {
						response["DomainInfo"] = map[string]string{"Domain": "example.com"}
					} else if action == "DescribeRecordList" {
						list := []any{}
						for id, v := range records {
							var number int
							fmt.Sscan(id, &number)
							list = append(list, map[string]any{"RecordId": number, "Name": "_acme-challenge", "Type": "TXT", "Value": v})
						}
						response["RecordList"] = list
					} else if action == "CreateRecord" {
						records["3"] = body["Value"].(string)
						created++
						response["RecordId"] = 3
					} else if action == "DeleteRecord" {
						if body["RecordId"] != float64(3) {
							t.Error("deleted unrelated record")
						}
						delete(records, "3")
						deleted++
					} else {
						t.Errorf("unexpected action %s", action)
					}
					send(map[string]any{"Response": response})
				case "alidns":
					q := r.URL.Query()
					action := q.Get("Action")
					response := map[string]any{"RequestId": "request"}
					if action == "DescribeDomainInfo" {
						response["DomainName"] = "example.com"
					} else if action == "DescribeSubDomainRecords" {
						list := []any{}
						for id, v := range records {
							list = append(list, map[string]string{"RecordId": id, "DomainName": "example.com", "RR": "_acme-challenge", "Type": "TXT", "Value": v})
						}
						response["DomainRecords"] = map[string]any{"Record": list}
					} else if action == "AddDomainRecord" {
						records["3"] = q.Get("Value")
						created++
						response["RecordId"] = "3"
					} else if action == "DeleteDomainRecord" {
						if q.Get("RecordId") != "3" {
							t.Error("deleted unrelated record")
						}
						delete(records, "3")
						deleted++
					} else {
						t.Errorf("unexpected action %s", action)
					}
					send(response)
				}
			}))
			defer server.Close()
			c := New(model.DNSCredential{Provider: provider, ID: "id", Secret: "secret"})
			c.api.client = server.Client()
			c.api.endpoints = endpoints{server.URL, server.URL, server.URL, server.URL}
			borrowed, err := c.PresentTXT(context.Background(), "_acme-challenge.example.com", "existing-value")
			if err != nil || borrowed.Owned {
				t.Fatal("existing TXT value ownership stolen", err)
			}
			if err = c.CleanTXT(context.Background(), borrowed); err != nil {
				t.Fatal(err)
			}
			owned, err := c.PresentTXT(context.Background(), "_acme-challenge.example.com", "new-value")
			if err != nil || !owned.Owned {
				t.Fatal("TXT creation failed", err)
			}
			records["3"] = "externally-changed"
			if err = c.CleanTXT(context.Background(), owned); err == nil {
				t.Fatal("cleanup deleted changed record")
			}
			records["3"] = "new-value"
			if err = c.CleanTXT(context.Background(), owned); err != nil {
				t.Fatal(err)
			}
			if created != 1 || deleted != 1 || len(records) != 2 || records["1"] != "unrelated" || records["2"] != "existing-value" {
				t.Fatal("TXT RRset was modified")
			}
		})
	}
}
