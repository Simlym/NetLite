package dnstask

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

// Golden values independently computed using Python hashlib/hmac and RFC3986 encoding.
func TestTencentSignature(t *testing.T) {
	req, _ := http.NewRequest("POST", "https://dnspod.tencentcloudapi.com/", nil)
	signTencent(req, "id", "dummy-secret", "DescribeDomain", []byte(`{"Domain":"example.com"}`), time.Unix(1791676800, 0))
	want := "Signature=8b9a142dfa7695311659376ef154b57b7abe25d1e682a3a4baf6e906df5a633c"
	if !strings.HasSuffix(req.Header.Get("Authorization"), want) || req.Header.Get("X-TC-Timestamp") != "1791676800" { t.Fatal("Tencent signing vector mismatch") }
}

func TestAliSignatureEscaping(t *testing.T) {
	values := url.Values{"Action": {"AddDomainRecord"}, "RR": {"*"}, "DomainName": {"example.com"}, "Value": {"192.0.2.2"}, "Line": {"默认"}, "SignatureNonce": {"test"}, "Timestamp": {"2026-10-11T00:00:00Z"}}
	if got := aliSignature(values, "dummy-secret"); got != "cVbKxkTJqLDcK+CTjg4DsWjZArY=" { t.Fatalf("AliDNS signing vector mismatch: %s", got) }
}
