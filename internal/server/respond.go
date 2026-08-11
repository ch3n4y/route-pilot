package server

import (
	"strconv"

	"github.com/gin-gonic/gin"
)

func ok(c *gin.Context, data any)      { c.JSON(200, data) }
func created(c *gin.Context, data any) { c.JSON(201, data) }
func noContent(c *gin.Context)         { c.Status(204) }
func fail(c *gin.Context, status int, msg string) {
	c.JSON(status, gin.H{"error": msg})
}

// atou 解析路由参数为 uint，失败返回 0（调用方据此 400）。
func atou(s string) uint {
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return 0
	}
	return uint(n)
}
