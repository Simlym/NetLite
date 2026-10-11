"""Generate a build-only overlay; never edit the user's upstream checkout."""
import json,sys
from pathlib import Path
source=Path(sys.argv[1]).resolve();stage=Path(sys.argv[2]).resolve();stage.mkdir(parents=True,exist_ok=True)
replacements={}
def rewrite(relative,old,new):
 path=source/relative;s=path.read_text(encoding="utf-8")
 if s.count(old)!=1:raise RuntimeError("Upstream bridge hook changed: "+relative)
 out=stage/(relative.replace("/","-")+".txt");out.write_text(s.replace(old,new),encoding="utf-8")
 replacements[str(path)]=str(out)
rewrite("config/config.go","return util.CreateHTTPClientWithInterface(conf.HttpInterface)","return NetLiteObserveClient(util.CreateHTTPClientWithInterface(conf.HttpInterface),conf.DNS.Name)")
rewrite("config/domains.go","}\n\n// checkParseDomains", "NetLiteIP(*domains,dnsConf)\n}\n\n// checkParseDomains")
path=source/"dns/index.go";s=path.read_text(encoding="utf-8")
for old,new in [("dnsSelected.Init(&dc", "config.NetLiteBegin(&dc)\n\t\tdnsSelected.Init(&dc"),("domains := dnsSelected.AddUpdateDomainRecords()","domains := dnsSelected.AddUpdateDomainRecords()\n\t\tconfig.NetLiteEnd(domains,&dc)")]:
 if s.count(old)!=1:raise RuntimeError("Upstream cycle hook changed")
 s=s.replace(old,new)
out=stage/"dns-index.go.txt";out.write_text(s,encoding="utf-8");replacements[str(path)]=str(out)
bridge=stage/"bridge.go.txt";bridge.write_text((Path(__file__).parent/"ddns-go-bridge/bridge.go.tmpl").read_text(encoding="utf-8"),encoding="utf-8")
replacements[str(source/"config/netlite_bridge.go")]=str(bridge)
test=stage/"bridge_test.go.txt";test.write_text((Path(__file__).parent/"ddns-go-bridge/bridge_test.go.tmpl").read_text(encoding="utf-8"),encoding="utf-8")
replacements[str(source/"config/netlite_bridge_test.go")]=str(test)
(stage/"overlay.json").write_text(json.dumps({"Replace":replacements}),encoding="utf-8")
