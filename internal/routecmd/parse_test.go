package routecmd

import "testing"

// 模拟本机（中文 locale）route print -4 输出：表头、活动路由、持久路由。
const sample = `===========================================================================
接口列表
 14...00 15 5d 0c 9a 08 ......Intel(R) I211 Gigabit Network Connection
  1...........................Software Loopback Interface 1
===========================================================================
IPv4 路由表
===========================================================================
活动路由:
        网络目标          网络掩码          网关       接口   跃点数
          0.0.0.0          0.0.0.0     192.168.1.1    192.168.1.9     25
       10.99.0.0      255.255.0.0     192.168.1.2    192.168.1.9     11
===========================================================================
持久路由:
  网络目标          网络掩码          网关       跃点数
     10.99.0.0      255.255.0.0     192.168.1.2       1
===========================================================================
`

func TestParseRoutePrint4(t *testing.T) {
	active, persistent, err := ParseRoutePrint4([]byte(sample))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(active) != 2 {
		t.Fatalf("active routes = %d, want 2", len(active))
	}
	if active[1].Dest != "10.99.0.0" || active[1].Mask != "255.255.0.0" ||
		active[1].Gateway != "192.168.1.2" || active[1].Metric != 11 {
		t.Fatalf("active[1] = %+v", active[1])
	}
	if len(persistent) != 1 || persistent[0].Gateway != "192.168.1.2" {
		t.Fatalf("persistent = %+v", persistent)
	}
}

// 含 on-link（本地化文字"在链路上"）的行：网关应标为 on-link，metric 取到。
const sampleOnLink = `===========================================================================
活动路由:
        网络目标          网络掩码          网关       接口   跃点数
       10.99.0.0      255.255.0.0      在链路上     192.168.1.9     11
===========================================================================
`

func TestParseOnLink(t *testing.T) {
	active, _, err := ParseRoutePrint4([]byte(sampleOnLink))
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(active) != 1 || active[0].Gateway != "on-link" || active[0].Metric != 11 {
		t.Fatalf("got %+v", active)
	}
}
