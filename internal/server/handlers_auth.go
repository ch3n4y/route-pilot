package server

import (
	"github.com/gin-gonic/gin"

	"route-manager/internal/auth"
	"route-manager/internal/models"
)

func (s *Server) hSetupStatus(c *gin.Context) {
	ok(c, gin.H{"needs_setup": !auth.IsSetup(s.db)})
}

func (s *Server) hSetup(c *gin.Context) {
	var body struct {
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.Password == "" {
		fail(c, 400, "密码不能为空")
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
	ok(c, gin.H{"password_set": auth.IsSetup(s.db), "elevated": s.elevated})
}
