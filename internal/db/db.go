package db

import (
	"path/filepath"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"route-manager/internal/config"
	"route-manager/internal/models"
)

// Open 打开/创建 SQLite 数据库，AutoMigrate，建立单活动网关部分唯一索引，seed 默认 settings。
func Open(cfg *config.AppConfig) (*gorm.DB, error) {
	path := filepath.Join(cfg.DataDir, "RouteManager.db")
	gdb, err := gorm.Open(sqlite.Open(path+"?_pragma=foreign_keys(1)"), &gorm.Config{})
	if err != nil {
		return nil, err
	}
	if err := gdb.AutoMigrate(&models.Segment{}, &models.Gateway{}, &models.Binding{},
		&models.Setting{}, &models.AuthToken{}, &models.AppliedRoute{}); err != nil {
		return nil, err
	}
	// 单活动网关约束（SQLite 部分唯一索引）
	if err := gdb.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_bindings_one_active_per_segment
		ON bindings (segment_id) WHERE is_active = 1`).Error; err != nil {
		return nil, err
	}
	// seed 默认 settings
	for k, v := range map[string]string{"host": cfg.Host, "port": "8080", "sync_on_change": "1"} {
		if GetSetting(gdb, k, "") == "" {
			_ = SetSetting(gdb, k, v)
		}
	}
	return gdb, nil
}

func GetSetting(gdb *gorm.DB, key, fallback string) string {
	var s models.Setting
	if err := gdb.First(&s, "key = ?", key).Error; err != nil {
		return fallback
	}
	return s.Value
}

func SetSetting(gdb *gorm.DB, key, val string) error {
	var s models.Setting
	if err := gdb.First(&s, "key = ?", key).Error; err != nil {
		return gdb.Create(&models.Setting{Key: key, Value: val}).Error
	}
	return gdb.Model(&s).Update("value", val).Error
}
