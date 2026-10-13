package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"netlite/internal/auth"
	"netlite/internal/store"
	"os"
	"path/filepath"
	"strings"
)

func command(args []string) error {
	f := flag.NewFlagSet(args[0], flag.ContinueOnError)
	listen := f.String("listen", "", "management address for info")
	data := f.String("data", "/var/lib/netlite", "private data directory")
	name := f.String("username", "admin", "administrator username")
	temporary := f.Bool("temporary", false, "require password change after login")
	if err := f.Parse(args[1:]); err != nil {
		return err
	}
	if f.NArg() != 0 {
		return fmt.Errorf("unexpected arguments")
	}
	if err := os.MkdirAll(*data, 0700); err != nil {
		return err
	}
	s, err := store.Open(filepath.Join(*data, "netlite.db"))
	if err != nil {
		return err
	}
	defer s.DB.Close()
	manager, err := auth.Open(s.DB, "")
	if err != nil {
		return err
	}
	switch args[0] {
	case "init-admin", "reset-password":
		fmt.Fprintln(os.Stderr, "从标准输入读取密码（12–72 字节）；不会保存明文密码。")
		scanner := bufio.NewScanner(os.Stdin)
		if !scanner.Scan() {
			return fmt.Errorf("需要通过标准输入提供密码")
		}
		password := strings.TrimSuffix(scanner.Text(), "\r")
		if args[0] == "init-admin" {
			err = manager.Create(*name, password, *temporary)
		} else {
			err = manager.Reset(password, *temporary)
		}
		if err != nil {
			return err
		}
		fmt.Println("管理员密码已设置；旧会话已失效。")
	case "info":
		ready, err := manager.Initialized()
		if err != nil {
			return err
		}
		fmt.Printf("管理员已初始化：%t\n", ready)
		if ready {
			var username string
			if err = s.DB.QueryRow("SELECT name FROM admin_account WHERE id=1").Scan(&username); err != nil {
				return err
			}
			fmt.Println("管理员：", username)
		} else {
			code, err := os.ReadFile(filepath.Join(*data, "admin-token"))
			if err != nil {
				return fmt.Errorf("请先启动服务生成初始化码，或使用 init-admin")
			}
			fmt.Println("初始化码：", strings.TrimSpace(string(code)))
		}
		var body string
		if err := s.DB.QueryRow("SELECT body FROM panel_settings WHERE id=1").Scan(&body); err == nil {
			var setting struct {
				Enabled bool   `json:"enabled"`
				Domain  string `json:"domain"`
			}
			if json.Unmarshal([]byte(body), &setting) == nil {
				fmt.Printf("面板 HTTPS：%t；域名：%s\n", setting.Enabled, setting.Domain)
			}
		}
		address := *listen
		if address == "" {
			if env, err := os.ReadFile("/etc/netlite/netlite.env"); err == nil {
				for _, line := range strings.Split(string(env), "\n") {
					if strings.HasPrefix(line, "NETLITE_LISTEN=") {
						address = strings.TrimPrefix(line, "NETLITE_LISTEN=")
					}
				}
			}
		}
		if address == "" {
			address = "127.0.0.1:18780"
		}
		fmt.Println("监听地址：", address)
		if host, port, err := net.SplitHostPort(address); err == nil {
			var setting struct {
				Enabled bool   `json:"enabled"`
				Domain  string `json:"domain"`
			}
			if json.Unmarshal([]byte(body), &setting) == nil && setting.Enabled {
				fmt.Println("访问地址：https://" + net.JoinHostPort(setting.Domain, port))
			} else if host == "0.0.0.0" || host == "::" {
				fmt.Println("访问地址：http://<服务器 IP>:" + port)
			} else {
				fmt.Println("访问地址：http://" + address)
			}
		}
	case "reset-https":
		_, err := s.DB.Exec("UPDATE panel_settings SET body='{}' WHERE id=1")
		if err != nil {
			return err
		}
		fmt.Println("已停用面板 HTTPS。执行 sudo systemctl restart netlite 后恢复 HTTP 访问。")
	default:
		return fmt.Errorf("unknown command")
	}
	return nil
}
