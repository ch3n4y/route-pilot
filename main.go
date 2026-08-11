package main

import (
	"flag"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/gin-gonic/gin"

	"route-manager/internal/config"
	"route-manager/internal/db"
	"route-manager/internal/elevate"
	"route-manager/internal/server"
	"route-manager/internal/sync"
)

var version = "1.0.0"

func main() {
	dev := flag.Bool("dev", false, "dev mode: serve from disk, allow vite CORS")
	noElevate := flag.Bool("no-elevate", false, "skip elevation (read-only)")
	console := flag.Bool("console", false, "keep console output instead of log file")
	flag.Parse()

	// 无控制台（release）时写日志文件
	if !*console {
		if lf, err := openLog(); err == nil {
			log.SetOutput(lf)
		}
	}

	// 提权：双击启动的非管理员实例 -> 弹 UAC 重启动
	if !*dev && !*noElevate && !elevate.IsElevated() {
		log.Println("未提权，尝试以管理员重启…")
		if err := elevate.RelaunchElevated(); err != nil {
			log.Println("提权失败，进入只读模式:", err)
		} else {
			return // 提权实例已启动
		}
	}
	elevated := elevate.IsElevated()

	gin.SetMode(gin.ReleaseMode)
	cfg := config.Load(*dev)

	gdb, err := db.Open(cfg)
	if err != nil {
		log.Fatal("数据库打开失败: ", err)
	}

	eng := sync.New(gdb)
	go eng.Reconcile() // 启动全量重放（防重启丢路由）

	addr := cfg.Host + ":" + db.GetSetting(gdb, "port", "8080")
	if !*dev {
		sf, _ := staticFS()
		r := server.New(gdb, eng, cfg, version, elevated, sf)
		openBrowserSoon("http://127.0.0.1:" + db.GetSetting(gdb, "port", "8080"))
		if err := r.Run(addr); err != nil {
			log.Fatal(err)
		}
		return
	}
	// dev 模式：不内嵌、不开浏览器，仅 API（前端走 vite :5173）
	r := server.New(gdb, eng, cfg, version, elevated, nil)
	log.Println("dev mode: 前端请启动 web 下的 vite (npm run dev) -> http://localhost:5173")
	if err := r.Run(addr); err != nil {
		log.Fatal(err)
	}
}

// openLog 打开 exe 旁 RouteManager.log。
func openLog() (*os.File, error) {
	dir := filepath.Dir(os.Args[0])
	if _, err := os.Stat(dir); err != nil {
		dir, _ = os.Getwd()
	}
	return os.OpenFile(filepath.Join(dir, "RouteManager.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
}

// openBrowserSoon 等服务就绪后自动打开浏览器（fire-and-forget）。
func openBrowserSoon(url string) {
	go func() {
		time.Sleep(900 * time.Millisecond)
		openBrowser(url)
	}()
}
