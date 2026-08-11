package netutil

import (
	"fmt"
	"net"
)

// CanonicalCIDR 规范化用户输入：10.1.2.3/8 -> 网络地址 10.0.0.0/8 + 掩码 255.0.0.0。
// 仅 IPv4；拒绝格式错误与 IPv6。
func CanonicalCIDR(s string) (network string, netmask string, err error) {
	ip, ipnet, err := net.ParseCIDR(s)
	if err != nil {
		return "", "", fmt.Errorf("CIDR 格式错误: %s", s)
	}
	if ip.To4() == nil {
		return "", "", fmt.Errorf("仅支持 IPv4: %s", s)
	}
	n := ipnet.IP.To4()
	m := net.IP(ipnet.Mask)
	return fmt.Sprintf("%d.%d.%d.%d/%d", n[0], n[1], n[2], n[3], ones(ipnet.Mask)),
		fmt.Sprintf("%d.%d.%d.%d", m[0], m[1], m[2], m[3]), nil
}

// SplitCIDR 把规范 CIDR 拆成 dest + mask（route 命令参数），如 10.0.0.0/8 -> ("10.0.0.0","255.0.0.0")。
func SplitCIDR(cidr string) (dest string, mask string, err error) {
	_, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return "", "", err
	}
	ip := ipnet.IP.To4()
	if ip == nil {
		return "", "", fmt.Errorf("仅支持 IPv4: %s", cidr)
	}
	m := net.IP(ipnet.Mask)
	return fmt.Sprintf("%d.%d.%d.%d", ip[0], ip[1], ip[2], ip[3]),
		fmt.Sprintf("%d.%d.%d.%d", m[0], m[1], m[2], m[3]), nil
}

func ones(mask net.IPMask) int {
	n := 0
	for _, b := range mask {
		for ; b > 0; b <<= 1 {
			n++
		}
	}
	return n
}

// Overlaps 判断两个 CIDR 是否重叠。
func Overlaps(a, b string) bool {
	_, na, err1 := net.ParseCIDR(a)
	_, nb, err2 := net.ParseCIDR(b)
	if err1 != nil || err2 != nil {
		return false
	}
	return na.Contains(nb.IP) || nb.Contains(na.IP)
}
