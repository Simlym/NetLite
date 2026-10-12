package model
import "testing"
func TestCertificateAccountChoice(t *testing.T){
 base:=Certificate{ID:"cert",Domains:[]string{"*.example.com"},Mode:"dns",Email:"admin@example.com",AccountID:"account"}
 if err:=(Config{Certificates:[]Certificate{base}}).Validate();err!=nil{t.Fatal(err)}
 for _,edit:=range []func(*Certificate){func(c *Certificate){c.AccountID=""},func(c *Certificate){c.AccountID="bad.id"},func(c *Certificate){c.Email=""},func(c *Certificate){c.Mode="acme"},func(c *Certificate){c.RenewBeforeDays=61}}{c:=base;edit(&c);if (Config{Certificates:[]Certificate{c}}).Validate()==nil{t.Fatal("invalid policy accepted")}}
}
