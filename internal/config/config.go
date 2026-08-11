package config

import (
	"os"
	"path/filepath"
)

type AppConfig struct {
	Host    string
	Port    int
	DataDir string
	Dev     bool
}

// Load 返回应用配置。数据目录优先 exe 所在目录（可写时），否则退回 %LOCALAPPDATA%\RouteManager。
func Load(dev bool) *AppConfig {
	dir := filepath.Dir(os.Args[0])
	if _, err := os.Stat(dir); err != nil {
		dir, _ = os.Getwd()
	}
	dataDir := dir
	f, err := os.Create(filepath.Join(dir, ".write_test"))
	if err != nil {
		dataDir = filepath.Join(os.Getenv("LOCALAPPDATA"), "RouteManager")
		_ = os.MkdirAll(dataDir, 0o755)
	} else {
		f.Close()
		_ = os.Remove(filepath.Join(dir, ".write_test"))
	}
	return &AppConfig{Host: "0.0.0.0", Port: 8080, DataDir: dataDir, Dev: dev}
}
