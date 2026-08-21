package server

import (
	"io/fs"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"route-manager/internal/config"
	"route-manager/internal/db"
	"route-manager/internal/sync"
)

type Server struct {
	db       *gorm.DB
	eng      *sync.Engine
	cfg      *config.AppConfig
	version  string
	elevated bool
	port     string // 实际运行端口（端口被占用时自动 +1，设置页展示用）
}

// requestSync 遵循“变更自动同步”设置，且只在管理员模式写系统路由。
func (s *Server) requestSync() {
	if s.elevated && db.GetSetting(s.db, "sync_on_change", "1") == "1" {
		s.eng.RequestSync()
	}
}

func (s *Server) reconcileAfterSwitch() sync.Result {
	if db.GetSetting(s.db, "sync_on_change", "1") == "1" {
		return s.eng.Reconcile()
	}
	return s.eng.Status()
}

// New 组装 gin 引擎。port 为实际运行端口（设置页展示用）；static 为内嵌前端（dev 模式传 nil）。
func New(gdb *gorm.DB, eng *sync.Engine, cfg *config.AppConfig, ver string, elevated bool, port string, static fs.FS) *gin.Engine {
	s := &Server{db: gdb, eng: eng, cfg: cfg, version: ver, elevated: elevated, port: port}
	r := gin.Default()
	// 管理程序不部署在反向代理后；禁用默认的“信任所有代理”，避免伪造
	// X-Forwarded-For 绕过首次设置的本机来源限制。
	_ = r.SetTrustedProxies(nil)

	if cfg.Dev {
		r.Use(corsLocalhost())
	}

	api := r.Group("/api")
	api.GET("/health", func(c *gin.Context) { ok(c, gin.H{"ok": true, "version": ver}) })
	api.GET("/me", s.hMe)
	api.GET("/segments", s.hSegments)
	api.POST("/segments", s.hCreateSegment)
	api.PUT("/segments/:id", s.hUpdateSegment)
	api.DELETE("/segments/:id", s.hDeleteSegment)
	api.POST("/segments/:id/switch", s.hSwitchSegment)
	api.POST("/segments/batch-switch", s.hBatchSwitch)
	api.GET("/gateways", s.hGateways)
	api.POST("/gateways", s.hCreateGateway)
	api.PUT("/gateways/:id", s.hUpdateGateway)
	api.DELETE("/gateways/:id", s.hDeleteGateway)
	api.GET("/network/interfaces", s.hNetworkInterfaces)
	api.GET("/bindings", s.hBindings)
	api.POST("/bindings", s.hCreateBinding)
	api.PUT("/bindings/:id", s.hUpdateBinding)
	api.DELETE("/bindings/:id", s.hDeleteBinding)
	api.POST("/bindings/set-active", s.hSetActive)
	api.GET("/routes/status", s.hRouteStatus)
	api.POST("/routes/sync", s.hRouteSync)
	api.POST("/routes/resolve-conflicts", s.hResolveConflicts)
	api.POST("/routes/delete", s.hDeleteActualRoute)
	api.POST("/routes/clear-persistent", s.hClearPersistent)
	api.GET("/routes/actual", s.hRouteActual)
	api.GET("/settings", s.hSettings)

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
