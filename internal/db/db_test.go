package db

import (
	"testing"

	"route-manager/internal/config"
	"route-manager/internal/models"
)

func TestOneActivePerSegmentIndex(t *testing.T) {
	gdb, err := Open(&config.AppConfig{DataDir: t.TempDir()})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	seg := models.Segment{Name: "s", Cidr: "10.0.0.0/8", Netmask: "255.0.0.0"}
	if err := gdb.Create(&seg).Error; err != nil {
		t.Fatal(err)
	}
	g1 := models.Gateway{Name: "g1", GatewayIP: "192.168.1.2"}
	g2 := models.Gateway{Name: "g2", GatewayIP: "192.168.1.3"}
	gdb.Create(&g1)
	gdb.Create(&g2)

	if err := gdb.Create(&models.Binding{SegmentID: seg.ID, GatewayID: g1.ID, IsActive: true}).Error; err != nil {
		t.Fatalf("first active: %v", err)
	}
	// 第二个活动绑定应被部分唯一索引拒绝
	if err := gdb.Create(&models.Binding{SegmentID: seg.ID, GatewayID: g2.ID, IsActive: true}).Error; err == nil {
		t.Fatal("expected partial unique index to reject second active binding")
	}
	// 先清活动，再加第二个活动，应成功
	if err := gdb.Model(&models.Binding{}).Where("segment_id = ?", seg.ID).Update("is_active", false).Error; err != nil {
		t.Fatal(err)
	}
	if err := gdb.Create(&models.Binding{SegmentID: seg.ID, GatewayID: g2.ID, IsActive: true}).Error; err != nil {
		t.Fatalf("second active after clear: %v", err)
	}
	sqlDB, _ := gdb.DB()
	_ = sqlDB.Close() // 释放文件句柄，允许 TempDir 清理
}

// 注：glebarez/sqlite 不生成 FK 约束，绑定级联删除与存在性校验由应用层（handlers）保证，
// 见 internal/server 的删除/绑定 handler。
