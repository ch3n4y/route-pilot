package main

import (
	"context"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
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

	// 单实例守卫（预检）：已有实例在运行则通知其打开界面并立即退出。
	// 放在提权之前——重复启动直接退出，不再弹 UAC。
	if anotherInstanceRunning() {
		log.Println("已有实例在运行，退出")
		notifyOpenUI()
		return
	}

	cfg := config.Load(*dev)

	// 无控制台（release）时写日志文件
	if !*console {
		if lf, err := openLog(cfg.DataDir); err == nil {
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

	// 提权后成为唯一运行实例：持有互斥量。
	// 并发首启竞态（两个实例同时通过预检）下若已被先到的实例抢到，这里会返回 false，同样退出。
	if !holdInstanceMutex() {
		log.Println("已有实例在运行，退出")
		notifyOpenUI()
		return
	}
	defer releaseInstanceMutex()

	gin.SetMode(gin.ReleaseMode)

	gdb, err := db.Open(cfg)
	if err != nil {
		log.Fatal("数据库打开失败: ", err)
	}

	eng := sync.New(gdb)
	if elevated {
		go eng.Reconcile() // 仅管理员模式启动全量重放，避免只读模式产生无意义错误
	}

	// 端口自动检测：从默认端口开始尝试监听，被占用则 +1，直到成功。
	port := config.DefaultPort
	ln, err := listenPort(cfg.Host, port)
	for err != nil {
		p, convErr := strconv.Atoi(port)
		if convErr != nil || p >= 65535 {
			log.Fatal("无可用端口（已尝试至 65535）: ", err)
		}
		port = strconv.Itoa(p + 1)
		ln, err = listenPort(cfg.Host, port)
	}
	trayPort = port
	log.Println("监听地址: " + cfg.Host + ":" + port)

	if *dev || *console {
		// 控制台/dev 模式：服务在前台，退出用 Ctrl+C
		r := server.New(gdb, eng, cfg, version, elevated, port, nil)
		if *dev {
			log.Println("dev mode: 前端请启动 web 下的 vite (npm run dev) -> http://localhost:5173")
		}
		if err := runHTTPServer(ln, r, nil); err != nil {
			log.Fatal(err)
		}
		return
	}

	// GUI 模式：托盘常驻主循环，HTTP 服务在后台；退出经托盘
	r := server.New(gdb, eng, cfg, version, elevated, port, mustStatic())
	go watchOpenUI() // 重复启动时由另一实例通知打开界面
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		openBrowserSoon("http://127.0.0.1:" + port)
		if err := runHTTPServer(ln, r, ctx); err != nil {
			log.Println("HTTP 服务异常: ", err)
			cancel()
		}
	}()
	runTray(func() {
		cancel()
	})
	<-ctx.Done()
	log.Println("已退出")
}

// runHTTPServer 在已绑定的监听器上启动 HTTP 服务；ctx 非 nil 时支持优雅关闭。
func runHTTPServer(ln net.Listener, handler http.Handler, ctx context.Context) error {
	srv := &http.Server{
		Addr: ln.Addr().String(), Handler: handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	if ctx == nil {
		return srv.Serve(ln)
	}
	go func() {
		<-ctx.Done()
		shCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = srv.Shutdown(shCtx)
	}()
	return srv.Serve(ln)
}

// listenPort 尝试在 host:port 上监听；端口被占用时返回错误，由调用方 +1 重试。
func listenPort(host, port string) (net.Listener, error) {
	return net.Listen("tcp", host+":"+port)
}

// openLog 在数据目录中打开 RouteManager.log。
func openLog(dataDir string) (*os.File, error) {
	return os.OpenFile(filepath.Join(dataDir, "RouteManager.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
}

// openBrowserSoon 等服务就绪后自动打开浏览器（fire-and-forget）。
func openBrowserSoon(url string) {
	go func() {
		time.Sleep(900 * time.Millisecond)
		if err := openBrowser(url); err != nil {
			log.Println("无法自动打开浏览器:", err)
		}
	}()
}
