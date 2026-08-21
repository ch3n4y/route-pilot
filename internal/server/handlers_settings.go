package server

import (
	"github.com/gin-gonic/gin"

	"route-manager/internal/db"
)

// hSettings 返回只读系统信息。运行端口由启动时自动检测决定（被占用则 +1），
// 端口等运行设置已不在 web 端提供修改入口。
func (s *Server) hSettings(c *gin.Context) {
	ok(c, gin.H{
		"port":           s.port,
		"host":           s.cfg.Host,
		"data_dir":       s.cfg.DataDir,
		"version":        s.version,
		"elevated":       s.elevated,
		"sync_on_change": db.GetSetting(s.db, "sync_on_change", "1"),
	})
}
