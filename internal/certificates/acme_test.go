package certificates

import(
 "context"
 "crypto/rand"
 "crypto/x509"
 "crypto/x509/pkix"
 "encoding/base64"
 "encoding/json"
 "encoding/pem"
 "fmt"
 "io"
 "math/big"
 "net/http"
 "net/http/httptest"
 "netlite/internal/dnsprovider"
 "netlite/internal/model"
 "strings"
 "sync"
 "testing"
 "time"
)
type transportFunc func(*http.Request)(*http.Response,error)
func(f transportFunc)RoundTrip(r *http.Request)(*http.Response,error){return f(r)}
func TestACMEProtocolHTTPAndDNS(t *testing.T){
 for _,mode:=range []string{"acme","dns"}{t.Run(mode,func(t *testing.T){
  m:=testManager(t);var mu sync.Mutex;records:=map[string]string{"unrelated":"unrelated-token"};nextRecord:=0
  credentials:=model.DNSCredential{Provider:"cloudflare",Secret:"fake-token"}
  transport:=transportFunc(func(r *http.Request)(*http.Response,error){mu.Lock();defer mu.Unlock();var result any
   switch{case r.URL.Path=="/client/v4/zones":result=[]any{map[string]string{"id":"zone","name":"example.com","status":"active"}}
   case r.Method=="GET":list:=[]any{};for id,value:=range records{list=append(list,map[string]string{"id":id,"name":"_acme-challenge.example.com","type":"TXT","content":value})};result=list
   case r.Method=="POST":var body struct{Content string};json.NewDecoder(r.Body).Decode(&body);nextRecord++;id:=fmt.Sprint(nextRecord);records[id]=body.Content;result=map[string]string{"id":id}
   case r.Method=="DELETE":parts:=strings.Split(r.URL.Path,"/");delete(records,parts[len(parts)-1]);result=map[string]string{"id":"removed"}
   default:t.Errorf("unexpected DNS request %s",r.Method)}
   b,_:=json.Marshal(map[string]any{"success":true,"result":result});return &http.Response{StatusCode:200,Header:make(http.Header),Body:io.NopCloser(strings.NewReader(string(b)))},nil
  })
  m.ResolveAccount=func(string)(model.DNSCredential,error){return credentials,nil};m.DNSClient=func(a model.DNSCredential)*dnsprovider.Client{return dnsprovider.NewWithHTTP(a,&http.Client{Transport:transport})}
  m.lookupCNAME=func(_ context.Context,name string)(string,error){return name,nil}
  m.lookupTXT=func(context.Context,string)([]string,error){mu.Lock();defer mu.Unlock();var values []string;for _,v:=range records{values=append(values,v)};return values,nil}
  var fixtureMu sync.Mutex;var domains []string;var issued []byte;validated:=map[string]bool{};accounts,orders:=0,0
  issuerKey,_,_:=newKey();issuer:=&x509.Certificate{SerialNumber:big.NewInt(100),Subject:pkix.Name{CommonName:"Mock ACME CA"},NotBefore:time.Now().Add(-time.Hour),NotAfter:time.Now().Add(24*time.Hour),IsCA:true,BasicConstraintsValid:true,KeyUsage:x509.KeyUsageCertSign}
  issuerDER,_:=x509.CreateCertificate(rand.Reader,issuer,issuer,issuerKey.Public(),issuerKey)
  server:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
 fixtureMu.Lock();defer fixtureMu.Unlock();base:="http://"+r.Host;
   w.Header().Set("Replay-Nonce",base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprint(time.Now().UnixNano()))));w.Header().Set("Content-Type","application/json")
   var payload map[string]json.RawMessage
   if r.Method=="POST"{var jws struct{Payload string};json.NewDecoder(r.Body).Decode(&jws);b,_:=base64.RawURLEncoding.DecodeString(jws.Payload);if len(b)>0{json.Unmarshal(b,&payload)}}
   send:=func(v any){json.NewEncoder(w).Encode(v)}
   identifiers:=func()[]map[string]string{var ids []map[string]string;for _,d:=range domains{ids=append(ids,map[string]string{"type":"dns","value":d})};return ids}
   order:=func(status string)map[string]any{auths:=[]string{};for i:=range domains{auths=append(auths,base+fmt.Sprintf("/auth/%d",i))};return map[string]any{"status":status,"identifiers":identifiers(),"authorizations":auths,"finalize":base+"/finalize","certificate":base+"/certificate"}}
   switch{
   case r.URL.Path=="/dir":send(map[string]string{"newNonce":base+"/nonce","newAccount":base+"/account","newOrder":base+"/orders","revokeCert":base+"/revoke","keyChange":base+"/key"})
   case r.URL.Path=="/nonce":w.WriteHeader(200)
   case r.URL.Path=="/account":accounts++;w.Header().Set("Location",base+"/account/1");w.WriteHeader(201);send(map[string]any{"status":"valid","orders":base+"/orders"})
   case r.URL.Path=="/orders":var ids []struct{Value string};json.Unmarshal(payload["identifiers"],&ids);domains=nil;for _,id:=range ids{domains=append(domains,id.Value)};validated=map[string]bool{};orders++;w.Header().Set("Location",base+"/order/1");w.WriteHeader(201);send(order("pending"))
   case strings.HasPrefix(r.URL.Path,"/auth/"):var i int;fmt.Sscanf(r.URL.Path,"/auth/%d",&i);status:="pending";if validated[fmt.Sprint(i)]{status="valid"};kind:="http-01";if mode=="dns"{kind="dns-01"};send(map[string]any{"status":status,"identifier":map[string]string{"type":"dns","value":strings.TrimPrefix(domains[i],"*.")},"wildcard":strings.HasPrefix(domains[i],"*."),"challenges":[]any{map[string]string{"type":kind,"url":base+fmt.Sprintf("/challenge/%d",i),"status":status,"token":fmt.Sprintf("token-%d-%d",orders,i)}}})
   case strings.HasPrefix(r.URL.Path,"/challenge/"):var i int;fmt.Sscanf(r.URL.Path,"/challenge/%d",&i)
    if mode=="acme"{req:=httptest.NewRequest("GET",fmt.Sprintf("/.well-known/acme-challenge/token-%d-%d",orders,i),nil);req.Host=domains[i];response:=httptest.NewRecorder();m.HTTP.ServeHTTP(response,req);if response.Code!=200||!strings.HasPrefix(response.Body.String(),fmt.Sprintf("token-%d-%d.",orders,i)){t.Error("HTTP-01 proof missing")}}
    validated[fmt.Sprint(i)]=true;send(map[string]string{"type":mode,"url":base+r.URL.Path,"status":"valid"})
   case r.URL.Path=="/finalize":var encoded string;json.Unmarshal(payload["csr"],&encoded);der,_:=base64.RawURLEncoding.DecodeString(encoded);csr,err:=x509.ParseCertificateRequest(der);if err!=nil{t.Error(err);w.WriteHeader(400);return};if csr.CheckSignature()!=nil{t.Error("invalid CSR signature")};leaf:=&x509.Certificate{SerialNumber:big.NewInt(int64(orders)),Subject:csr.Subject,DNSNames:csr.DNSNames,NotBefore:time.Now().Add(-time.Minute),NotAfter:time.Now().Add(time.Hour),KeyUsage:x509.KeyUsageDigitalSignature,ExtKeyUsage:[]x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
    leafDER,err:=x509.CreateCertificate(rand.Reader,leaf,issuer,csr.PublicKey,issuerKey);if err!=nil{t.Error(err)};issued=append(pem.EncodeToMemory(&pem.Block{Type:"CERTIFICATE",Bytes:leafDER}),pem.EncodeToMemory(&pem.Block{Type:"CERTIFICATE",Bytes:issuerDER})...);send(order("valid"))
   case r.URL.Path=="/certificate":w.Header().Set("Content-Type","application/pem-certificate-chain");w.Write(issued)
   default:t.Errorf("unexpected ACME endpoint %s",r.URL.Path);w.WriteHeader(404)
   }
  }));defer server.Close();m.acmeDirectory=server.URL+"/dir";m.acmeHTTP=server.Client()
  c:=model.Certificate{ID:"public",Domains:[]string{"example.com"},Mode:mode,Email:"admin@example.com",Staging:true};if mode=="dns"{c.AccountID="account";c.Domains=append(c.Domains,"*.example.com")}
  bundle,err:=m.issue(context.Background(),c);if err!=nil{t.Fatal(err)};if _,err=checkBundle(c,bundle,time.Now());err!=nil{t.Fatal(err)}
  if _,err=m.issue(context.Background(),c);err!=nil{t.Fatal(err)};fixtureMu.Lock();if accounts!=1||orders!=2{t.Fatalf("account reuse failed: %d accounts %d orders",accounts,orders)};fixtureMu.Unlock()
  mu.Lock();if len(records)!=1||records["unrelated"]!="unrelated-token"{t.Error("TXT cleanup modified unrelated value")};mu.Unlock()
  var pending int;m.db.DB.QueryRow("SELECT count(*) FROM certificate_txt").Scan(&pending);if pending!=0{t.Fatal("completed authorization left cleanup journal")}
 })}
}
