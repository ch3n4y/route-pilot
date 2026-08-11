package sync

import "testing"

func TestClassify(t *testing.T) {
	des := []DesiredEntry{
		{SegmentID: 1, Cidr: "10.0.0.0/8", GatewayIP: "192.168.1.2"},
		{SegmentID: 2, Cidr: "172.16.0.0/16", GatewayIP: "192.168.1.3"},
		{SegmentID: 3, Cidr: "10.5.0.0/16", GatewayIP: "192.168.1.4"},
	}
	actual := map[string]string{
		"10.0.0.0/8":    "192.168.1.2", // 一致
		"172.16.0.0/16": "192.168.1.99", // 网关不同 -> CONFLICT
		// 10.5.0.0/16 缺失
	}
	got := classify(des, actual)
	if got[0].Status != "OK" {
		t.Fatalf("want OK got %+v", got[0])
	}
	if got[1].Status != "CONFLICT" {
		t.Fatalf("want CONFLICT got %+v", got[1])
	}
	if got[2].Status != "MISSING" {
		t.Fatalf("want MISSING got %+v", got[2])
	}
}
