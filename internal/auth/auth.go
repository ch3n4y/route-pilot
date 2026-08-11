package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"route-manager/internal/models"
)

const tokenTTL = 30 * 24 * time.Hour

func HashPassword(pw string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	return string(b), err
}

func VerifyPassword(hash, pw string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(pw)) == nil
}

func IsSetup(gdb *gorm.DB) bool {
	return dbSetting(gdb, "admin_password", "") != ""
}

func dbSetting(gdb *gorm.DB, key, fb string) string {
	var s models.Setting
	if err := gdb.First(&s, "key = ?", key).Error; err != nil {
		return fb
	}
	return s.Value
}

func CreateToken(gdb *gorm.DB) (string, time.Time, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", time.Time{}, err
	}
	token := hex.EncodeToString(raw)
	exp := time.Now().Add(tokenTTL)
	hash := sha256Hex(token)
	return token, exp, gdb.Create(&models.AuthToken{TokenHash: hash, ExpiresAt: exp}).Error
}

func Login(gdb *gorm.DB, pw string) (string, time.Time, bool) {
	hash := dbSetting(gdb, "admin_password", "")
	if hash == "" || !VerifyPassword(hash, pw) {
		return "", time.Time{}, false
	}
	token, exp, err := CreateToken(gdb)
	return token, exp, err == nil
}

func RevokeToken(gdb *gorm.DB, token string) error {
	return gdb.Delete(&models.AuthToken{}, "token_hash = ?", sha256Hex(token)).Error
}

func Middleware(gdb *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.GetHeader("Authorization")
		token := strings.TrimPrefix(h, "Bearer ")
		if token == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
			return
		}
		var t models.AuthToken
		if err := gdb.First(&t, "token_hash = ? AND expires_at > ?", sha256Hex(token), time.Now()).Error; err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "登录已过期"})
			return
		}
		c.Set("token", token)
		c.Next()
	}
}

func sha256Hex(s string) string {
	b := sha256.Sum256([]byte(s))
	return hex.EncodeToString(b[:])
}
