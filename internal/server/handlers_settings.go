package server

import (
	"github.com/gin-gonic/gin"

	"route-manager/internal/auth"
	"route-manager/internal/db"
)

func (s *Server) hSettings(c *gin.Context) {
	ok(c, gin.H{
		"port":           db.GetSetting(s.db, "port", "8080"),
		"host":           db.GetSetting(s.db, "host", s.cfg.Host),
		"data_dir":       s.cfg.DataDir,
		"version":        s.version,
		"elevated":       s.elevated,
		"sync_on_change": db.GetSetting(s.db, "sync_on_change", "1"),
	})
}

func (s *Server) hUpdateSettings(c *gin.Context) {
	var body struct {
		Port          string `json:"port"`
		SyncOnChange  string `json:"sync_on_change"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		fail(c, 400, "参数错误")
		return
	}
	restart := false
	if body.Port != "" && body.Port != db.GetSetting(s.db, "port", "8080") {
		if err := db.SetSetting(s.db, "port", body.Port); err != nil {
			fail(c, 500, err.Error())
			return
		}
		restart = true
	}
	if body.SyncOnChange == "0" || body.SyncOnChange == "1" {
		_ = db.SetSetting(s.db, "sync_on_change", body.SyncOnChange)
	}
	ok(c, gin.H{"restart_required": restart})
}

func (s *Server) hChangePassword(c *gin.Context) {
	var body struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.NewPassword == "" {
		fail(c, 400, "参数错误")
		return
	}
	cur := db.GetSetting(s.db, "admin_password", "")
	if !auth.VerifyPassword(cur, body.OldPassword) {
		fail(c, 401, "旧密码错误")
		return
	}
	hash, err := auth.HashPassword(body.NewPassword)
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	if err := db.SetSetting(s.db, "admin_password", hash); err != nil {
		fail(c, 500, err.Error())
		return
	}
	noContent(c)
}
