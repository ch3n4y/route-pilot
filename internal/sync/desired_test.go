package sync

import (
	"testing"

	"route-manager/internal/config"
	"route-manager/internal/db"
	"route-manager/internal/models"
)

func TestDesiredQuery(t *testing.T) {
	gdb, err := db.Open(&config.AppConfig{DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { sqlDB, _ := gdb.DB(); sqlDB.Close() }()

	seg := models.Segment{Name: "办公网", Cidr: "10.0.0.0/8", Netmask: "255.0.0.0", Enabled: true}
	segOff := models.Segment{Name: "禁用段", Cidr: "172.16.0.0/16", Netmask: "255.255.0.0", Enabled: false}
	gdb.Create(&seg)
	gdb.Create(&segOff)
	gw := models.Gateway{Name: "GW-LAN", GatewayIP: "192.168.1.2", Metric: 1}
	gwOff := models.Gateway{Name: "GW-OFF", GatewayIP: "192.168.1.3", Enabled: false}
	gdb.Create(&gw)
	gdb.Create(&gwOff)
	gdb.Create(&models.Binding{SegmentID: seg.ID, GatewayID: gw.ID, IsActive: true, Enabled: true})
	gdb.Create(&models.Binding{SegmentID: segOff.ID, GatewayID: gw.ID, IsActive: true, Enabled: true}) // 段禁用 -> 不计入

	rows, err := desired(gdb)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Cidr != "10.0.0.0/8" || rows[0].GatewayIP != "192.168.1.2" {
		t.Fatalf("got %+v", rows)
	}
}
