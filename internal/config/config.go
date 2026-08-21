package config

import (
	"io"
	"os"
	"path/filepath"
)

type AppConfig struct {
	Host    string
	Port    int
	DataDir string
	Dev     bool
}

// DefaultPort 默认监听端口（用户在部署时指定的固定端口）。
const DefaultPort = "38254"

// Load 返回应用配置。数据始终放在用户目录，不在 exe/当前运行目录产生数据库。
func Load(dev bool) *AppConfig {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		base, _ = os.UserConfigDir()
	}
	if base == "" {
		base = os.TempDir()
	}
	dirName := "RouteManager"
	if dev {
		dirName = "RouteManager-dev"
	}
	dataDir := filepath.Join(base, dirName)
	_ = os.MkdirAll(dataDir, 0o700)
	if !dev {
		migrateLegacyDatabase(dataDir)
	}
	return &AppConfig{Host: "127.0.0.1", Port: 38254, DataDir: dataDir, Dev: dev}
}

// migrateLegacyDatabase 首次升级时复制 exe 旁的旧数据库；旧文件保留作备份。
func migrateLegacyDatabase(dataDir string) {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	sourcePath := filepath.Join(filepath.Dir(exe), "RouteManager.db")
	targetPath := filepath.Join(dataDir, "RouteManager.db")
	if filepath.Clean(sourcePath) == filepath.Clean(targetPath) {
		return
	}
	if _, err := os.Stat(targetPath); err == nil {
		return
	}
	source, err := os.Open(sourcePath)
	if err != nil {
		return
	}
	defer source.Close()
	target, err := os.OpenFile(targetPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	if _, err = io.Copy(target, source); err == nil {
		err = target.Sync()
	}
	closeErr := target.Close()
	if err != nil || closeErr != nil {
		_ = os.Remove(targetPath)
	}
}
