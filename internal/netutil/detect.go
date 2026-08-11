package netutil

import (
	"encoding/json"
	"net"
	"os/exec"
	"syscall"
)

type InterfaceInfo struct {
	Index          int      `json:"index"`
	Name           string   `json:"name"`
	IPs            []string `json:"ips"`
	DefaultGateway string   `json:"default_gateway"`
}

// LocalInterfaces 枚举本机网卡（仅 IPv4 地址），用于网关表单候选建议（非数据来源）。
func LocalInterfaces() ([]InterfaceInfo, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	gw := defaultGatewayMap()
	var out []InterfaceInfo
	for _, it := range ifaces {
		if it.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, _ := it.Addrs()
		info := InterfaceInfo{Index: it.Index, Name: it.Name}
		for _, a := range addrs {
			ipn, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			if ipn.IP.To4() != nil {
				info.IPs = append(info.IPs, ipn.IP.String())
			}
		}
		if len(info.IPs) > 0 {
			info.DefaultGateway = gw[it.Index]
			out = append(out, info)
		}
	}
	return out, nil
}

// GatewayReachableOnInterface 返回网关 IP 所在的本地接口 index（0 = 不在任何本机子网）。
func GatewayReachableOnInterface(gw string) (int, bool) {
	ip := net.ParseIP(gw)
	if ip == nil || ip.To4() == nil {
		return 0, false
	}
	ifaces, err := net.Interfaces()
	if err != nil {
		return 0, false
	}
	for _, it := range ifaces {
		addrs, _ := it.Addrs()
		for _, a := range addrs {
			ipn, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			if ipn.Contains(ip) {
				return it.Index, true
			}
		}
	}
	return 0, false
}

type gwEntry struct {
	Index int
	GW    string
}

// defaultGatewayMap 用 PowerShell Get-NetIPConfiguration 获取各接口的默认网关。
// PowerShell 输出是标准 JSON（locale 无关），优于解析本地化的 ipconfig 文本。
func defaultGatewayMap() map[int]string {
	m := map[int]string{}
	ps := `@(Get-NetIPConfiguration | Where-Object {$_.IPv4DefaultGateway} | ForEach-Object {
	  [PSCustomObject]@{ Index = $_.InterfaceIndex; GW = $_.IPv4DefaultGateway.NextHop }
	}) | ConvertTo-Json`
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", ps)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.Output()
	if err != nil {
		return m
	}
	var list []gwEntry
	if err := json.Unmarshal(out, &list); err != nil {
		var single gwEntry
		if json.Unmarshal(out, &single) == nil && single.Index != 0 {
			m[single.Index] = single.GW
		}
		return m
	}
	for _, e := range list {
		if e.Index != 0 && e.GW != "" {
			m[e.Index] = e.GW
		}
	}
	return m
}
