package sync

import "testing"

func TestClassify(t *testing.T) {
	des := []DesiredEntry{
		{SegmentID: 1, Cidr: "10.0.0.0/8", GatewayIP: "192.168.1.2", Metric: 1},
		{SegmentID: 2, Cidr: "172.16.0.0/16", GatewayIP: "192.168.1.3", Metric: 1},
		{SegmentID: 3, Cidr: "10.5.0.0/16", GatewayIP: "192.168.1.4", Metric: 2},
	}
	actual := map[string]routeActual{
		routeKey("10.0.0.0/8", "192.168.1.2"):     {Gateway: "192.168.1.2", Metric: 1, MetricKnown: true},
		routeKey("172.16.0.0/16", "192.168.1.99"): {Gateway: "192.168.1.99", Metric: 1, MetricKnown: true},
		routeKey("10.5.0.0/16", "192.168.1.4"):    {Gateway: "192.168.1.4", Metric: 1, MetricKnown: true},
	}
	got := classify(des, actual)
	if got[0].Status != "OK" {
		t.Fatalf("want OK got %+v", got[0])
	}
	if got[1].Status != "MISSING" {
		t.Fatalf("different next hop should be added as an additional route, got %+v", got[1])
	}
	if got[2].Status != "MISMATCH" {
		t.Fatalf("want MISMATCH got %+v", got[2])
	}
}

func TestClassifySkipsMetricComparisonWhenOnlyRoutePrintIsAvailable(t *testing.T) {
	des := []DesiredEntry{{SegmentID: 1, Cidr: "10.0.0.0/8", GatewayIP: "192.168.1.2", Metric: 1}}
	actual := map[string]routeActual{
		// route print may show the route metric plus interface metric, so an
		// unavailable Get-NetRoute result must not cause a destructive rewrite.
		routeKey("10.0.0.0/8", "192.168.1.2"): {Gateway: "192.168.1.2", Metric: 11},
	}
	got := classify(des, actual)
	if got[0].Status != "OK" {
		t.Fatalf("unknown route metric should not mismatch: %+v", got[0])
	}
}

func TestApplyEffectiveMetricsUsesBindingPositionForGatewayPriority(t *testing.T) {
	rows := []DesiredEntry{
		{BindingID: 10, Cidr: "10.0.0.0/8", Metric: 1, Position: 1},
		{BindingID: 11, Cidr: "10.0.0.0/8", Metric: 1, Position: 0},
	}
	applyEffectiveMetrics(rows)
	if rows[0].Metric != 2 || rows[1].Metric != 1 {
		t.Fatalf("drag priority should map to ascending route metrics: %+v", rows)
	}
}

// TestApplyEffectiveMetrics 验证有效跃点按"包含关系"自动提升。
func TestApplyEffectiveMetrics(t *testing.T) {
	rows := []DesiredEntry{
		{SegmentID: 1, Cidr: "19.25.0.0/16", Metric: 1},
		{SegmentID: 2, Cidr: "19.25.22.0/24", Metric: 1},
	}
	applyEffectiveMetrics(rows)
	if rows[0].Metric != 2 || rows[1].Metric != 1 {
		t.Fatalf("含 /24 的 /16 应提升为 2，/24 保持 1: %+v", rows)
	}
	// 再加一层 /32：/16 -> 3，/24 -> 2（从基础跃点全新计算，避免重复提升）
	rows = []DesiredEntry{
		{SegmentID: 1, Cidr: "19.25.0.0/16", Metric: 1},
		{SegmentID: 2, Cidr: "19.25.22.0/24", Metric: 1},
		{SegmentID: 3, Cidr: "19.25.22.100/32", Metric: 1},
	}
	applyEffectiveMetrics(rows)
	if rows[0].Metric != 3 || rows[1].Metric != 2 || rows[2].Metric != 1 {
		t.Fatalf("三层包含应 3/2/1: %+v", rows)
	}
	// 删掉 /24 后 /16 恢复为 1
	rows = []DesiredEntry{{SegmentID: 1, Cidr: "19.25.0.0/16", Metric: 1}}
	applyEffectiveMetrics(rows)
	if rows[0].Metric != 1 {
		t.Fatalf("无被覆盖网段时应保持基础跃点: %+v", rows)
	}
}

func TestRouteKeyIncludesGateway(t *testing.T) {
	if routeKey("10.0.0.0/8", "192.168.1.2") == routeKey("10.0.0.0/8", "192.168.1.3") {
		t.Fatal("same CIDR with a changed gateway must be treated as a stale applied route")
	}
}
