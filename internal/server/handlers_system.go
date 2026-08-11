package server

import (
	"time"

	"github.com/gin-gonic/gin"
)

// hShutdown 触发程序退出：先返回响应，再在后台调用 onShutdown。
func (s *Server) hShutdown(c *gin.Context) {
	ok(c, gin.H{"ok": true, "msg": "正在退出程序…"})
	if s.onShutdown != nil {
		go func() {
			time.Sleep(300 * time.Millisecond)
			s.onShutdown()
		}()
	}
}
