package netutil

import "testing"

func TestCanonicalCIDR(t *testing.T) {
	n, m, err := CanonicalCIDR("10.1.2.3/8")
	if err != nil || n != "10.0.0.0/8" || m != "255.0.0.0" {
		t.Fatalf("got %q %q err=%v", n, m, err)
	}
	n, _, err = CanonicalCIDR("10.0.0.0/32")
	if err != nil || n != "10.0.0.0/32" {
		t.Fatalf("/32 got %q err=%v", n, err)
	}
	n, m, err = CanonicalCIDR("0.0.0.0/0")
	if err != nil || n != "0.0.0.0/0" || m != "0.0.0.0" {
		t.Fatalf("/0 got %q %q err=%v", n, m, err)
	}
	if _, _, err = CanonicalCIDR("fe80::1/64"); err == nil {
		t.Fatal("ipv6 should be rejected")
	}
	if _, _, err = CanonicalCIDR("not-a-cidr"); err == nil {
		t.Fatal("garbage should be rejected")
	}
}

func TestCanonicalHostIPv4(t *testing.T) {
	network, mask, err := CanonicalCIDR(" 192.168.27.10 ")
	if err != nil {
		t.Fatal(err)
	}
	if network != "192.168.27.10/32" || mask != "255.255.255.255" {
		t.Fatalf("got %s %s", network, mask)
	}
}

func TestOverlaps(t *testing.T) {
	if !Overlaps("10.0.0.0/8", "10.5.0.0/16") {
		t.Fatal("10.0.0.0/8 should overlap 10.5.0.0/16")
	}
	if Overlaps("10.0.0.0/8", "172.16.0.0/12") {
		t.Fatal("should not overlap")
	}
}

func TestContains(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"10.0.0.0/8", "10.5.0.0/16", true},
		{"10.0.0.0/8", "10.5.22.0/24", true},
		{"10.5.0.0/16", "10.0.0.0/8", false}, // b 比 a 更宽
		{"10.5.0.0/16", "172.16.0.0/12", false},
		{"10.0.0.0/8", "10.0.0.0/8", false}, // 相等不算严格包含
		{"0.0.0.0/0", "192.168.1.0/24", true},
	}
	for _, c := range cases {
		if got := Contains(c.a, c.b); got != c.want {
			t.Fatalf("Contains(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}
