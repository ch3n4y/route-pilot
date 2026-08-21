package server

import (
	"net"

	"github.com/gin-gonic/gin"

	"route-manager/internal/auth"
	"route-manager/internal/models"
)

func (s *Server) hSetupStatus(c *gin.Context) {
	ok(c, gin.H{"needs_setup": !auth.IsSetup(s.db)})
}

func (s *Server) hSetup(c *gin.Context) {
	clientIP := net.ParseIP(c.ClientIP())
	if clientIP == nil || !clientIP.IsLoopback() {
		fail(c, 403, "首次设置只能在运行程序的本机完成")
		return
	}
	var body struct {
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || len(body.Password) < 6 {
		fail(c, 400, "密码至少 6 位")
		return
	}
	if auth.IsSetup(s.db) {
		fail(c, 409, "已初始化")
		return
	}
	hash, err := auth.HashPassword(body.Password)
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	if err := s.db.Create(&models.Setting{Key: "admin_password", Value: hash}).Error; err != nil {
		fail(c, 500, err.Error())
		return
	}
	token, exp, err := auth.CreateToken(s.db)
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	created(c, gin.H{"token": token, "expires_at": exp})
}

func (s *Server) hLogin(c *gin.Context) {
	var body struct {
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		fail(c, 400, "参数错误")
		return
	}
	if !auth.IsSetup(s.db) {
		fail(c, 409, "请先完成首次设置")
		return
	}
	token, exp, okLogin := auth.Login(s.db, body.Password)
	if !okLogin {
		fail(c, 401, "密码错误")
		return
	}
	ok(c, gin.H{"token": token, "expires_at": exp})
}

func (s *Server) hLogout(c *gin.Context) {
	t := c.GetString("token")
	_ = auth.RevokeToken(s.db, t)
	noContent(c)
}

func (s *Server) hMe(c *gin.Context) {
	ok(c, gin.H{"elevated": s.elevated})
}
