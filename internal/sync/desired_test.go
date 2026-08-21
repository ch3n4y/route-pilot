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
	gw := models.Gateway{Name: "GW-LAN", GatewayIP: "192.168.1.2", Enabled: true}
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

// TestDesiredEffectiveMetrics 走真实 DB：期望态里被更具体网段覆盖的 /16 有效跃点应自动 +1。
func TestDesiredEffectiveMetrics(t *testing.T) {
	gdb, err := db.Open(&config.AppConfig{DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { sqlDB, _ := gdb.DB(); sqlDB.Close() }()

	segWide := models.Segment{Name: "19.25.0.0/16", Cidr: "19.25.0.0/16", Netmask: "255.255.0.0", Metric: 1, Enabled: true}
	segNarrow := models.Segment{Name: "19.25.22.0/24", Cidr: "19.25.22.0/24", Netmask: "255.255.255.0", Metric: 1, Enabled: true}
	segOther := models.Segment{Name: "192.168.0.0/24", Cidr: "192.168.0.0/24", Netmask: "255.255.255.0", Metric: 1, Enabled: true}
	gdb.Create(&segWide)
	gdb.Create(&segNarrow)
	gdb.Create(&segOther)
	gwA := models.Gateway{Name: "GW-A", GatewayIP: "192.168.1.2", Enabled: true}
	gwB := models.Gateway{Name: "GW-B", GatewayIP: "192.168.1.3", Enabled: true}
	gdb.Create(&gwA)
	gdb.Create(&gwB)
	// 两个嵌套网段走不同网关
	gdb.Create(&models.Binding{SegmentID: segWide.ID, GatewayID: gwA.ID, IsActive: true, Enabled: true})
	gdb.Create(&models.Binding{SegmentID: segNarrow.ID, GatewayID: gwB.ID, IsActive: true, Enabled: true})
	gdb.Create(&models.Binding{SegmentID: segOther.ID, GatewayID: gwA.ID, IsActive: true, Enabled: true})

	rows, err := desired(gdb)
	if err != nil {
		t.Fatal(err)
	}
	metric := map[string]int{}
	gw := map[string]string{}
	for _, r := range rows {
		metric[r.Cidr] = r.Metric
		gw[r.Cidr] = r.GatewayIP
	}
	if metric["19.25.0.0/16"] != 2 {
		t.Fatalf("被 /24 覆盖的 /16 有效跃点应为 2: %+v", metric)
	}
	if metric["19.25.22.0/24"] != 1 {
		t.Fatalf("/24 有效跃点应为 1: %+v", metric)
	}
	if metric["192.168.0.0/24"] != 1 {
		t.Fatalf("无覆盖关系的网段应保持基础跃点 1: %+v", metric)
	}
	if gw["19.25.0.0/16"] == gw["19.25.22.0/24"] {
		t.Fatalf("嵌套网段应各走各的网关: %+v", gw)
	}
}
