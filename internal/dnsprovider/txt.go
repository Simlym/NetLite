package dnsprovider

import (
 "context"
 "fmt"
 "net/url"
 "strconv"
 "strings"
)

// TXTRecord identifies one challenge value, not an entire TXT RRset.
// Owned=false denotes a pre-existing value which must never be removed.
type TXTRecord struct { Provider,Zone,Name,ID,Value string; Owned bool }

func zoneHost(name string)string{
 parts:=strings.Split(strings.TrimSuffix(name,"."),".")
 for len(parts)>1&&strings.Contains(parts[0],"_"){parts=parts[1:]}
 return strings.Join(parts,".")
}
func (c *Client) ListTXT(ctx context.Context,name string)([]TXTRecord,error){
 name=strings.ToLower(strings.TrimSuffix(name,".")); p:=&c.api
 var zone string;var err error
 switch p.task.Provider {
 case "cloudflare":zone,err=p.cfZone(ctx,zoneHost(name))
 case "dnspod":zone,err=p.podZone(ctx,zoneHost(name))
 case "tencentcloud":zone,err=p.tcZone(ctx,zoneHost(name))
 case "alidns":zone,err=p.aliZone(ctx,zoneHost(name))
 default:return nil,fmt.Errorf("unsupported DNS provider")
 }
 if err!=nil{return nil,err}
 var out []TXTRecord
 for page:=0;page<100;page++{
  var batch []TXTRecord
  add:=func(id,value string){batch=append(batch,TXTRecord{Provider:p.task.Provider,Zone:zone,Name:name,ID:id,Value:value})}
  switch p.task.Provider{
  case "cloudflare":
   var rows []struct{ID,Name,Type,Content string}
   _,err=p.cfCall(ctx,"GET","/zones/"+url.PathEscape(zone)+"/dns_records?"+url.Values{"type":{"TXT"},"name":{name},"per_page":{"100"},"page":{strconv.Itoa(page+1)}}.Encode(),nil,&rows)
   for _,r:=range rows{if r.Type!="TXT"||!strings.EqualFold(r.Name,name)||r.ID==""{return nil,fmt.Errorf("TXT record identity mismatch")};add(r.ID,r.Content)}
  case "dnspod":
   var result struct{Records []struct{ID,Name,Type,Value string}}
   err=p.podCall(ctx,"Record.List",url.Values{"domain":{zone},"sub_domain":{subdomain(name,zone)},"record_type":{"TXT"},"offset":{strconv.Itoa(page*100)},"length":{"100"}},false,&result)
   if errorCode(err)=="10"{err=nil;result.Records=[]struct{ID,Name,Type,Value string}{}}
   if err==nil&&result.Records==nil{return nil,fmt.Errorf("incomplete TXT record list")}
   for _,r:=range result.Records{if r.Type!="TXT"||!strings.EqualFold(r.Name,subdomain(name,zone))||r.ID==""{return nil,fmt.Errorf("TXT record identity mismatch")};add(r.ID,r.Value)}
  case "tencentcloud":
   var result struct{RecordList []struct{RecordId int64;Name,Type,Value string}}
   err=p.tcCall(ctx,"DescribeRecordList",map[string]any{"Domain":zone,"Subdomain":subdomain(name,zone),"RecordType":"TXT","Offset":page*100,"Limit":100},false,&result)
   empty:=errorCode(err)=="ResourceNotFound.NoDataOfRecord";if empty{err=nil}
   if err==nil&&result.RecordList==nil&&!empty{return nil,fmt.Errorf("incomplete TXT record list")}
   for _,r:=range result.RecordList{if r.Type!="TXT"||!strings.EqualFold(r.Name,subdomain(name,zone))||r.RecordId<=0{return nil,fmt.Errorf("TXT record identity mismatch")};add(strconv.FormatInt(r.RecordId,10),r.Value)}
  case "alidns":
   var result struct{DomainRecords struct{Record []struct{RecordId,DomainName,RR,Type,Value string}}}
   err=p.aliCall(ctx,"DescribeSubDomainRecords",url.Values{"DomainName":{zone},"SubDomain":{name},"Type":{"TXT"},"PageNumber":{strconv.Itoa(page+1)},"PageSize":{"100"}},false,&result)
   if err==nil&&result.DomainRecords.Record==nil{return nil,fmt.Errorf("incomplete TXT record list")}
   for _,r:=range result.DomainRecords.Record{if r.Type!="TXT"||!strings.EqualFold(r.DomainName,zone)||!strings.EqualFold(r.RR,subdomain(name,zone))||r.RecordId==""{return nil,fmt.Errorf("TXT record identity mismatch")};add(r.RecordId,r.Value)}
  }
  if err!=nil{return nil,err};out=append(out,batch...)
  if len(batch)<100{return out,nil}
 }
 return nil,fmt.Errorf("too many TXT records")
}

func (c *Client) PresentTXT(ctx context.Context,name,value string)(TXTRecord,error){
 rows,err:=c.ListTXT(ctx,name);if err!=nil{return TXTRecord{},err}
 for _,r:=range rows{if r.Value==value{return r,nil}}
 p:=&c.api;var zone string
 switch p.task.Provider{
 case "cloudflare":zone,err=p.cfZone(ctx,zoneHost(name))
 case "dnspod":zone,err=p.podZone(ctx,zoneHost(name))
 case "tencentcloud":zone,err=p.tcZone(ctx,zoneHost(name))
 case "alidns":zone,err=p.aliZone(ctx,zoneHost(name))
 }
 if err!=nil{return TXTRecord{},err}
 r:=TXTRecord{Provider:p.task.Provider,Zone:zone,Name:strings.TrimSuffix(name,"."),Value:value,Owned:true}
 switch p.task.Provider{
 case "cloudflare":
  var result struct{ID string};_,err=p.cfCall(ctx,"POST","/zones/"+url.PathEscape(zone)+"/dns_records",map[string]any{"type":"TXT","name":r.Name,"content":value,"ttl":120},&result);r.ID=result.ID
 case "dnspod":
  var result struct{Record struct{ID string}};err=p.podCall(ctx,"Record.Create",url.Values{"domain":{zone},"sub_domain":{subdomain(r.Name,zone)},"record_type":{"TXT"},"record_line":{"默认"},"value":{value},"ttl":{"600"}},true,&result);r.ID=result.Record.ID
 case "tencentcloud":
  var result struct{RecordId int64};err=p.tcCall(ctx,"CreateRecord",map[string]any{"Domain":zone,"SubDomain":subdomain(r.Name,zone),"RecordType":"TXT","RecordLine":"默认","Value":value,"TTL":600},true,&result);if result.RecordId>0{r.ID=strconv.FormatInt(result.RecordId,10)}
 case "alidns":
  var result struct{RecordId string};err=p.aliCall(ctx,"AddDomainRecord",url.Values{"DomainName":{zone},"RR":{subdomain(r.Name,zone)},"Type":{"TXT"},"Value":{value},"TTL":{"600"}},true,&result);r.ID=result.RecordId
 }
 if err==nil&&r.ID==""{err=fmt.Errorf("DNS provider did not confirm TXT record ID")}
 return r,err
}

func (c *Client) CleanTXT(ctx context.Context,r TXTRecord)error{
 if !r.Owned{return nil};if r.Provider!=c.api.task.Provider{return fmt.Errorf("TXT credential provider changed")}
 rows,err:=c.ListTXT(ctx,r.Name);if err!=nil{return err}
 var found *TXTRecord
 for i:=range rows{candidate:=rows[i];if r.ID!=""&&candidate.ID!=r.ID{continue};if candidate.Value!=r.Value{if candidate.ID==r.ID{return fmt.Errorf("TXT record changed; refusing cleanup")};continue};if found!=nil{return fmt.Errorf("ambiguous TXT cleanup")};found=&candidate}
 if found==nil{return nil}
 p:=&c.api
 switch p.task.Provider{
 case "cloudflare":_,err=p.cfCall(ctx,"DELETE","/zones/"+url.PathEscape(found.Zone)+"/dns_records/"+url.PathEscape(found.ID),nil,nil)
 case "dnspod":err=p.podCall(ctx,"Record.Remove",url.Values{"domain":{found.Zone},"record_id":{found.ID}},true,nil)
 case "tencentcloud":id,e:=strconv.ParseInt(found.ID,10,64);if e!=nil{return e};err=p.tcCall(ctx,"DeleteRecord",map[string]any{"Domain":found.Zone,"RecordId":id},true,nil)
 case "alidns":err=p.aliCall(ctx,"DeleteDomainRecord",url.Values{"RecordId":{found.ID}},true,nil)
 }
 return err
}
