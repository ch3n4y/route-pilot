package server

import (
	"io/fs"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"route-manager/internal/auth"
	"route-manager/internal/config"
	"route-manager/internal/sync"
)

type Server struct {
	db         *gorm.DB
	eng        *sync.Engine
	cfg        *config.AppConfig
	version    string
	elevated   bool
	onShutdown func()
}

// New 组装 gin 引擎。static 为内嵌前端（dev 模式传 nil），onShutdown 为"退出程序"时调用。
func New(gdb *gorm.DB, eng *sync.Engine, cfg *config.AppConfig, ver string, elevated bool, static fs.FS, onShutdown func()) *gin.Engine {
	s := &Server{db: gdb, eng: eng, cfg: cfg, version: ver, elevated: elevated, onShutdown: onShutdown}
	r := gin.Default()

	if cfg.Dev {
		r.Use(corsLocalhost())
	}

	api := r.Group("/api")
	api.GET("/health", func(c *gin.Context) { ok(c, gin.H{"ok": true, "version": ver}) })
	api.GET("/setup/status", s.hSetupStatus)
	api.POST("/setup", s.hSetup)
	api.POST("/login", s.hLogin)

	authed := api.Group("")
	authed.Use(auth.Middleware(gdb))
	authed.GET("/me", s.hMe)
	authed.POST("/logout", s.hLogout)
	authed.GET("/segments", s.hSegments)
	authed.POST("/segments", s.hCreateSegment)
	authed.PUT("/segments/:id", s.hUpdateSegment)
	authed.DELETE("/segments/:id", s.hDeleteSegment)
	authed.POST("/segments/:id/switch", s.hSwitchSegment)
	authed.POST("/segments/batch-switch", s.hBatchSwitch)
	authed.GET("/gateways", s.hGateways)
	authed.POST("/gateways", s.hCreateGateway)
	authed.PUT("/gateways/:id", s.hUpdateGateway)
	authed.DELETE("/gateways/:id", s.hDeleteGateway)
	authed.GET("/network/interfaces", s.hNetworkInterfaces)
	authed.GET("/bindings", s.hBindings)
	authed.POST("/bindings", s.hCreateBinding)
	authed.PUT("/bindings/:id", s.hUpdateBinding)
	authed.DELETE("/bindings/:id", s.hDeleteBinding)
	authed.POST("/bindings/set-active", s.hSetActive)
	authed.GET("/routes/status", s.hRouteStatus)
	authed.POST("/routes/sync", s.hRouteSync)
	authed.GET("/routes/actual", s.hRouteActual)
	authed.GET("/settings", s.hSettings)
	authed.PUT("/settings", s.hUpdateSettings)
	authed.PUT("/settings/password", s.hChangePassword)
	authed.POST("/system/shutdown", s.hShutdown)

	// 静态 + SPA 兜底（dev 模式 static 为 nil，前端走 vite）。
	// 不用 r.StaticFS("/")：根级 catch-all 会与 /api 路由冲突导致 gin panic。
	if static != nil {
		r.NoRoute(func(c *gin.Context) {
			p := strings.TrimPrefix(c.Request.URL.Path, "/")
			if strings.HasPrefix(p, "api/") {
				c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
				return
			}
			if p == "" || p == "index.html" {
				// 直接伺服 index.html（ServeFileFS 对 index.html 会 301 重定向，不适用）
				b, _ := fs.ReadFile(static, "index.html")
				c.Data(http.StatusOK, "text/html; charset=utf-8", b)
				return
			}
			if f, err := static.Open(p); err == nil {
				f.Close()
				http.ServeFileFS(c.Writer, c.Request, static, p)
				return
			}
			// SPA 深链接兜底
			b, _ := fs.ReadFile(static, "index.html")
			c.Data(http.StatusOK, "text/html; charset=utf-8", b)
		})
	}
	return r
}

// corsLocalhost 仅开发模式放行 vite 调试源。
func corsLocalhost() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin == "http://localhost:5173" || origin == "http://127.0.0.1:5173" {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type")
		}
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
