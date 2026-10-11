package dnstask

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Signing follows the provider APIs and ddns-go v6.17.7's signing implementation.
// Attribution and upstream MIT license are in THIRD_PARTY_NOTICES.md.
func hash256(s string) string { sum := sha256.Sum256([]byte(s)); return hex.EncodeToString(sum[:]) }
func mac256(key []byte, s string) []byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(s))
	return h.Sum(nil)
}
func signTencent(req *http.Request, id, secret, action string, payload []byte, now time.Time) {
	const host = "dnspod.tencentcloudapi.com"
	const headers = "content-type;host;x-tc-action"
	canonicalHeaders := "content-type:application/json\nhost:" + host + "\nx-tc-action:" + strings.ToLower(action) + "\n"
	canonical := "POST\n/\n\n" + canonicalHeaders + "\n" + headers + "\n" + hash256(string(payload))
	date := now.UTC().Format("2006-01-02")
	timestamp := strconv.FormatInt(now.Unix(), 10)
	scope := date + "/dnspod/tc3_request"
	toSign := "TC3-HMAC-SHA256\n" + timestamp + "\n" + scope + "\n" + hash256(canonical)
	key := mac256([]byte("TC3"+secret), date)
	key = mac256(key, "dnspod")
	key = mac256(key, "tc3_request")
	signature := hex.EncodeToString(mac256(key, toSign))
	req.Header.Set("Authorization", "TC3-HMAC-SHA256 Credential="+id+"/"+scope+", SignedHeaders="+headers+", Signature="+signature)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-TC-Action", action)
	req.Header.Set("X-TC-Timestamp", timestamp)
	req.Header.Set("X-TC-Version", "2021-03-23")
}

func percentEncode(s string) string { return strings.ReplaceAll(url.QueryEscape(s), "+", "%20") }
func aliSignature(values url.Values, secret string) string {
	canonical := strings.ReplaceAll(values.Encode(), "+", "%20")
	h := hmac.New(sha1.New, []byte(secret+"&"))
	h.Write([]byte("GET&%2F&" + percentEncode(canonical)))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}
func signAli(values url.Values, id, secret string, now time.Time) error {
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return err
	}
	values.Set("AccessKeyId", id)
	values.Set("SignatureMethod", "HMAC-SHA1")
	values.Set("SignatureVersion", "1.0")
	values.Set("SignatureNonce", hex.EncodeToString(nonce[:]))
	values.Set("Timestamp", now.UTC().Format("2006-01-02T15:04:05Z"))
	values.Set("Format", "JSON")
	values.Set("Version", "2015-01-09")
	values.Del("Signature")
	values.Set("Signature", aliSignature(values, secret))
	return nil
}
