package proxy
import(
 "context"
 "encoding/json"
 "net/http"
 "net/http/httptest"
 "netlite/internal/certificates"
 "netlite/internal/model"
 "netlite/internal/store"
 "os"
 "path/filepath"
 "strings"
 "testing"
)
func fixtureMaterial(t *testing.T,domains []string)model.CertificateFiles{
 t.Helper();dir:=t.TempDir();db,err:=store.Open(filepath.Join(dir,"db"));if err!=nil{t.Fatal(err)};defer db.DB.Close();m,err:=certificates.Open(context.Background(),db,filepath.Join(dir,"certs"),"127.0.0.1:0");if err!=nil{t.Fatal(err)}
 bundle,err:=m.Issue(context.Background(),model.Certificate{ID:"fixture",Domains:domains,Mode:"internal"});if err!=nil{t.Fatal(err)}
 files:=model.CertificateFiles{CertificateFile:filepath.Join(dir,"cert.pem"),PrivateKeyFile:filepath.Join(dir,"key.pem")};if err=os.WriteFile(files.CertificateFile,bundle.Certificate,0600);err!=nil{t.Fatal(err)};if err=os.WriteFile(files.PrivateKeyFile,bundle.PrivateKey,0600);err!=nil{t.Fatal(err)};return files
}
func proxyConfig()model.Config{return model.Config{Certificates:[]model.Certificate{{ID:"cert",Domains:[]string{"localhost"},Mode:"internal"}},Routes:[]model.Route{{ID:"plain",Domain:"plain.localhost",Scheme:"http",Port:19081,Upstream:"http://127.0.0.1:8080",Enabled:true},{ID:"secure",Domain:"localhost",Scheme:"https",Port:19444,Upstream:"https://localhost:8081",CertificateID:"cert",Enabled:true}}}}
func TestRenderProxyOnlyTLS(t *testing.T){
 c:=New("caddy",t.TempDir(),"127.0.0.1:20190");files:=fixtureMaterial(t,[]string{"localhost"});c.ResolveCertificate=func(model.Certificate)(model.CertificateFiles,error){return files,nil}
 b,err:=c.Render(proxyConfig());if err!=nil{t.Fatal(err)};var parsed any;if json.Unmarshal(b,&parsed)!=nil{t.Fatal("invalid JSON")}
 for _,s:=range []string{"load_files","any_tag",":19081",":19444","server_name","disable"}{if !strings.Contains(string(b),s){t.Fatal("missing",s)}}
 for _,s:=range []string{"automation","automate","issuers","pki","dns.providers","api_token","insecure_skip_verify","letsencrypt"}{if strings.Contains(string(b),s){t.Fatal("proxy owns certificate issuance",s)}}
}
func TestRejectOccupiedAdmin(t *testing.T){server:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){t.Error("existing service called")}));defer server.Close();c:=New("missing",t.TempDir(),strings.TrimPrefix(server.URL,"http://"));if err:=c.Run(context.Background());err==nil||!strings.Contains(err.Error(),"occupied"){t.Fatal("admin conflict not rejected",err)}}
func TestCertificateReloadUsesAppliedSnapshot(t *testing.T){
 binary:=os.Getenv("NETLITE_TEST_CADDY");if binary==""{t.Skip("real Caddy not configured")};binary,_=filepath.Abs(binary)
 var payload string;server:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){var value any;json.NewDecoder(r.Body).Decode(&value);b,_:=json.Marshal(value);payload=string(b);w.WriteHeader(200)}));defer server.Close()
 c:=New(binary,t.TempDir(),strings.TrimPrefix(server.URL,"http://"));files:=fixtureMaterial(t,[]string{"localhost"});c.ResolveCertificate=func(model.Certificate)(model.CertificateFiles,error){return files,nil}
 cfg:=proxyConfig();if err:=c.Apply(context.Background(),cfg);err!=nil{t.Fatal(err)}
 cfg.Routes[0].Enabled=false;files=fixtureMaterial(t,[]string{"localhost"})
 if err:=c.ReloadCertificates(context.Background());err!=nil{t.Fatal(err)}
 if !strings.Contains(payload,"plain.localhost")||!strings.Contains(payload,files.CertificateFile){t.Fatal("renewal activated draft or retained old certificate")}
 applied,err:=c.Applied();if err!=nil||!applied.Routes[0].Enabled{t.Fatal("applied snapshot changed")}
 if _,err=os.Stat(filepath.Join(c.Data,"proxy-applied.json"));err!=nil{t.Fatal(err)}
}
