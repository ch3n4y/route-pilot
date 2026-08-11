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

func TestOverlaps(t *testing.T) {
	if !Overlaps("10.0.0.0/8", "10.5.0.0/16") {
		t.Fatal("10.0.0.0/8 should overlap 10.5.0.0/16")
	}
	if Overlaps("10.0.0.0/8", "172.16.0.0/12") {
		t.Fatal("should not overlap")
	}
}
