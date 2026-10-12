package certificates

import(
 "context"
 "crypto/tls"
 "fmt"
 "net/http"
 "net/http/httptest"
 "netlite/internal/model"
 "netlite/internal/store"
 "os"
 "path/filepath"
 "strings"
 "testing"
 "time"

 "github.com/mholt/acmez/v3/acme"
)
func testManager(t *testing.T)*Manager{t.Helper();db,err:=store.Open(filepath.Join(t.TempDir(),"db.sqlite"));if err!=nil{t.Fatal(err)};t.Cleanup(func(){db.DB.Close()});m,err:=Open(context.Background(),db,filepath.Join(t.TempDir(),"certs"),"127.0.0.1:0");if err!=nil{t.Fatal(err)};t.Cleanup(m.HTTP.Close);return m}
func policy()model.Certificate{return model.Certificate{ID:"local",Domains:[]string{"localhost"},Mode:"internal"}}
func execute(t *testing.T,m *Manager,c model.Certificate){t.Helper();if err:=m.Request(c,true);err!=nil{t.Fatal(err)};m.execute(<-m.queue)}
func TestInternalCertificatePersistenceRenewalAndFailure(t *testing.T){
 m:=testManager(t);c:=policy();execute(t,m,c)
 first,err:=m.Files(c);if err!=nil{t.Fatal(err)};cert,err:=tls.LoadX509KeyPair(first.CertificateFile,first.PrivateKeyFile);if err!=nil||len(cert.Certificate)<2{t.Fatal("missing leaf or CA chain")}
 state,err:=m.Status(c);if err!=nil||state.Status!="ready"||!state.RenewAt.Before(state.NotAfter){t.Fatalf("state: %+v %v",state,err)}
 root,err:=m.RootCertificate();if err!=nil||strings.Contains(string(root),"PRIVATE KEY"){t.Fatal("root export contains private key")}
 secondManager,err:=Open(context.Background(),m.db,m.dir,"127.0.0.1:0");if err!=nil{t.Fatal(err)};persisted,err:=secondManager.Files(c);if err!=nil||persisted!=first{t.Fatal("restart lost material reference")}
 originalClock:=m.now;now:=state.RenewAt.Add(time.Minute);m.now=func()time.Time{return now}
 cfg,_:=m.db.Load();cfg.Certificates=[]model.Certificate{c};m.db.Save(cfg)
 if err=m.Sweep();err!=nil{t.Fatal(err)};select{case queued:=<-m.queue:m.execute(queued);default:t.Fatal("renewal not scheduled")}
 renewed,err:=m.Files(c);if err!=nil||renewed==first{t.Fatal("renewal did not publish a new generation")}
 m.now=originalClock;now=state.RenewAt.Add(2*time.Hour);m.now=func()time.Time{return now}
 m.Issue=func(context.Context,model.Certificate)(Bundle,error){return Bundle{},fmt.Errorf("secret=private-provider-response")}
 execute(t,m,c);current,_:=m.Files(c);if current!=renewed{t.Fatal("failed renewal replaced usable material")}
 failure,_:=m.Status(c);if failure.Status!="failed"||failure.Attempts!=1||!failure.NextAttempt.After(now)||strings.Contains(failure.LastError,"secret"){t.Fatalf("failure: %+v",failure)}
 if err=m.Request(c,true);err==nil{t.Fatal("manual request bypassed retry backoff")}
}
func TestInvalidBundleCannotReplaceCertificate(t *testing.T){m:=testManager(t);c:=policy();execute(t,m,c);old,_:=m.Files(c);r,_:=m.load(c);m.now=func()time.Time{return r.State.RenewAt.Add(time.Minute)};m.Issue=func(context.Context,model.Certificate)(Bundle,error){return Bundle{Certificate:[]byte("not PEM"),PrivateKey:[]byte("private")},nil};execute(t,m,c);current,_:=m.Files(c);if current!=old{t.Fatal("invalid material replaced previous pair")}}
func TestPolicyChangesAndAppliedCertificatesAreSeparate(t *testing.T){
 m:=testManager(t);old:=policy();execute(t,m,old);changed:=old;changed.Domains=[]string{"new.localhost"}
 if _,err:=m.Files(changed);err==nil{t.Fatal("changed policy reused old material")}
 cfg,_:=m.db.Load();cfg.Certificates=[]model.Certificate{changed};m.db.Save(cfg);m.Applied=func()(model.Config,error){return model.Config{Certificates:[]model.Certificate{old}},nil}
 list,err:=m.policies();if err!=nil||len(list)!=2{t.Fatal("lost applied policy after draft change")}
}
func TestAPIContainsNoPrivateMaterial(t *testing.T){
 m:=testManager(t);c:=policy();cfg,_:=m.db.Load();cfg.Certificates=[]model.Certificate{c};m.db.Save(cfg);execute(t,m,c)
 for _,path:=range []string{"/","/root","/local/certificate"}{w:=httptest.NewRecorder();m.Router().ServeHTTP(w,httptest.NewRequest("GET",path,nil));if w.Code!=200||strings.Contains(w.Body.String(),"PRIVATE KEY")||strings.Contains(w.Body.String(),"key.pem"){t.Fatalf("unsafe export %s: %s",path,w.Body.String())}}
 files,_:=m.Files(c);info,_:=os.Stat(files.PrivateKeyFile);if os.PathSeparator=='/'&&info.Mode().Perm()!=0600{t.Fatal("private key permissions")}
}
func TestHTTPChallengeHostBoundaryAndCleanup(t *testing.T){
 h:=NewHTTPChallenge("127.0.0.1:0");defer h.Close();c:=acme.Challenge{Token:"token",KeyAuthorization:"token.thumbprint",Identifier:acme.Identifier{Type:"dns",Value:"example.com"}}
 if err:=h.Present(context.Background(),c);err!=nil{t.Fatal(err)}
 for _,host:=range []string{"example.com","other.com"}{r:=httptest.NewRequest(http.MethodGet,c.HTTP01ResourcePath(),nil);r.Host=host;w:=httptest.NewRecorder();h.ServeHTTP(w,r);if host=="example.com"&&w.Body.String()!=c.KeyAuthorization{t.Fatal("challenge missing")};if host!="example.com"&&w.Code!=404{t.Fatal("wrong host accepted")}}
 h.CleanUp(context.Background(),c);w:=httptest.NewRecorder();r:=httptest.NewRequest("GET",c.HTTP01ResourcePath(),nil);r.Host="example.com";h.ServeHTTP(w,r);if w.Code!=404{t.Fatal("challenge retained after cleanup")}
}
