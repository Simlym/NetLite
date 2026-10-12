package certificates

import(
 "context"
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "fmt"
 "net/http"
  "netlite/internal/model"
 "os"
 "path/filepath"
 "time"

 "github.com/mholt/acmez/v3"
 "github.com/mholt/acmez/v3/acme"
)

type accountRecord struct{Account acme.Account;PrivateKey []byte}
func(m *Manager) issue(ctx context.Context,c model.Certificate)(Bundle,error){
 if c.Mode=="internal"{return m.issueInternal(ctx,c)}
 directory:="https://acme-v02.api.letsencrypt.org/directory";if c.Staging{directory="https://acme-staging-v02.api.letsencrypt.org/directory"};if m.acmeDirectory!=""{directory=m.acmeDirectory}
 httpClient:=m.acmeHTTP;if httpClient==nil{httpClient=&http.Client{Timeout:30*time.Second}}
 client:=&acmez.Client{Client:&acme.Client{Directory:directory,HTTPClient:httpClient,UserAgent:"NetLite",PollTimeout:5*time.Minute},ChallengeSolvers:map[string]acmez.Solver{}}
 if c.Mode=="dns"{
  if m.ResolveAccount==nil{return Bundle{},fmt.Errorf("DNS account resolver unavailable")};a,err:=m.ResolveAccount(c.AccountID);if err!=nil{return Bundle{},err}
  client.ChallengeSolvers[acme.ChallengeTypeDNS01]=&dnsSolver{manager:m,accountID:c.AccountID,provider:a.Provider,client:m.DNSClient(a),scope:PolicyKey(c)}
 }else{client.ChallengeSolvers[acme.ChallengeTypeHTTP01]=m.HTTP}
 hash:=sha256.Sum256([]byte(directory+"\n"+c.Email));accounts:=filepath.Join(m.dir,"accounts");if err:=privateDir(accounts);err!=nil{return Bundle{},err}
 path:=filepath.Join(accounts,hex.EncodeToString(hash[:])+".json")
 var saved accountRecord;b,err:=os.ReadFile(path)
 if err==nil{if err=json.Unmarshal(b,&saved);err!=nil{return Bundle{},err}}else if os.IsNotExist(err){_,saved.PrivateKey,err=newKey();if err!=nil{return Bundle{},err};saved.Account.Contact=[]string{"mailto:"+c.Email};saved.Account.TermsOfServiceAgreed=true;b,err=json.Marshal(saved);if err==nil{err=atomicWrite(path,b)};if err!=nil{return Bundle{},err}}else{return Bundle{},err}
 signer,err:=parseKey(saved.PrivateKey);if err!=nil{return Bundle{},err};saved.Account.PrivateKey=signer
 if saved.Account.Location==""||saved.Account.Status!=acme.StatusValid{saved.Account,err=client.NewAccount(ctx,saved.Account);if err!=nil{return Bundle{},err};b,err=json.Marshal(saved);if err==nil{err=atomicWrite(path,b)};if err!=nil{return Bundle{},err}}
 key,keyPEM,err:=newKey();if err!=nil{return Bundle{},err}
 chains,err:=client.ObtainCertificateForSANs(ctx,saved.Account,key,c.Domains);if err!=nil{return Bundle{},err};if len(chains)==0{return Bundle{},fmt.Errorf("ACME returned no certificate")}
 return Bundle{Certificate:chains[0].ChainPEM,PrivateKey:keyPEM},nil
}
