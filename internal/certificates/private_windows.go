//go:build windows
package certificates
import("fmt";"os/exec";"os/user")
func protect(path string)error{
 current,err:=user.Current();if err!=nil{return fmt.Errorf("cannot identify certificate owner")}
 if exec.Command("icacls",path,"/inheritance:r","/grant:r","*"+current.Uid+":(OI)(CI)F").Run()!=nil{return fmt.Errorf("cannot protect certificate directory")};return nil
}
