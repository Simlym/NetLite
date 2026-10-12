//go:build !windows
package certificates
import "os"
func protect(path string)error{return os.Chmod(path,0700)}
