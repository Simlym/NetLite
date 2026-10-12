package proxy
import("testing";"netlite/internal/model")
func TestDraftAndAppliedPreview(t *testing.T){
 c:=New("unused",t.TempDir(),"127.0.0.1:20190")
 view,err:=c.Configuration(model.Config{});if err!=nil||view["draft"]==nil||view["applied"]!=nil{t.Fatal("incorrect initial view",err)}
 cfg:=model.Config{Routes:[]model.Route{{ID:"app",Domain:"example.com",Scheme:"https",Port:18443,CertificateID:"cert",Enabled:true}},Certificates:[]model.Certificate{{ID:"cert",Domains:[]string{"example.com"},Mode:"internal"}}}
 view,err=c.Configuration(cfg);if err!=nil||view["draft_error"]==nil||view["draft"]!=nil{t.Fatal("missing certificate must be visible without hiding applied view",err)}
}
