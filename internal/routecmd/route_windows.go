//go:build windows

package routecmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"os/exec"
	"strings"
	"syscall"
)

const routeExe = `C:\Windows\System32\route.exe`

func runRoute(args ...string) (string, error) {
	cmd := exec.Command(routeExe, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	err := cmd.Run()
	if err != nil {
		msg := strings.TrimSpace(errb.String())
		if msg == "" {
			msg = err.Error()
		}
		return out.String(), fmt.Errorf("%s", msg)
	}
	return out.String(), nil
}

// Add 添加活动路由：route add <dest> MASK <mask> <gw> [METRIC n] [IF idx]。
// 不写入 PersistentStore；重启后由应用启动时全量重放恢复。
func Add(gw, dest, mask string, metric, ifIndex int) error {
	args := []string{"add", dest, "MASK", mask, gw}
	if metric > 0 {
		args = append(args, "METRIC", fmt.Sprintf("%d", metric))
	}
	if ifIndex > 0 {
		args = append(args, "IF", fmt.Sprintf("%d", ifIndex))
	}
	_, err := runRoute(args...)
	return err
}

// Delete 删除路由；先试 -p（含持久），失败回退普通 delete。
func Delete(gw, dest, mask string) error {
	_, err := runRoute("-p", "delete", dest, "MASK", mask, gw)
	if err == nil {
		return nil
	}
	_, err = runRoute("delete", dest, "MASK", mask, gw)
	return err
}

// DeleteFromStore 从指定 Windows 路由存储精确删除路由。store 为 active 或 persistent。
func DeleteFromStore(cidr, gateway, store string) error {
	ip, network, err := net.ParseCIDR(cidr)
	if err != nil || ip.To4() == nil {
		return fmt.Errorf("无效 IPv4 网段: %s", cidr)
	}
	prefix := network.String()
	nextHop := gateway
	if gateway == "on-link" {
		nextHop = "0.0.0.0"
	}
	if parsed := net.ParseIP(nextHop); parsed == nil || parsed.To4() == nil {
		return fmt.Errorf("无效下一跳: %s", gateway)
	}
	if store == "persistent" {
		return deletePersistentRoute(network.String(), gateway)
	}
	policyStore := "ActiveStore"
	if store != "active" {
		return fmt.Errorf("无效路由存储: %s", store)
	}
	// prefix/nextHop/store 均已严格校验，可安全拼入 PowerShell 命令。
	script := fmt.Sprintf("[Console]::OutputEncoding=[Text.Encoding]::UTF8; Remove-NetRoute -DestinationPrefix '%s' -NextHop '%s' -PolicyStore %s -Confirm:$false -ErrorAction Stop", prefix, nextHop, policyStore)
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("%s", msg)
	}
	return nil
}

// deletePersistentRoute 兼容旧版 Windows Server。其 PersistentStore 路由常带 InterfaceIndex=0，
// Remove-NetRoute 会报系统错误 87，因此使用 route.exe 删除；若系统连活动副本也一并删除，
// 再按原参数恢复为非持久活动路由。
func deletePersistentRoute(cidr, gateway string) error {
	activeBefore, _, err := ReadRoutes()
	if err != nil {
		return err
	}
	var restore *RouteRow
	restoreMetric, restoreIfIndex := 0, 0
	for i := range activeBefore {
		if activeBefore[i].Cidr() == cidr && activeBefore[i].Gateway == gateway {
			copy := activeBefore[i]
			restore = &copy
			break
		}
	}
	if restore != nil {
		restoreMetric, restoreIfIndex = activeRouteParameters(cidr, gateway)
	}
	dest, mask, err := cidrDestMask(cidr)
	if err != nil {
		return err
	}
	if err := Delete(gateway, dest, mask); err != nil {
		return err
	}
	activeAfter, persistentAfter, err := ReadRoutes()
	if err != nil {
		return err
	}
	if routeExists(persistentAfter, cidr, gateway) {
		return fmt.Errorf("持久路由仍然存在")
	}
	if restore != nil && !routeExists(activeAfter, cidr, gateway) {
		if restoreIfIndex == 0 {
			restoreIfIndex = interfaceIndexForIP(restore.Interface)
		}
		if restoreMetric <= 0 {
			restoreMetric = restore.Metric
		}
		if err := Add(gateway, dest, mask, restoreMetric, restoreIfIndex); err != nil {
			return fmt.Errorf("持久路由已删除，但恢复活动路由失败: %w", err)
		}
	}
	return nil
}

func activeRouteParameters(cidr, gateway string) (metric, ifIndex int) {
	nextHop := gateway
	if gateway == "on-link" {
		nextHop = "0.0.0.0"
	}
	script := fmt.Sprintf("[Console]::OutputEncoding=[Text.Encoding]::UTF8; Get-NetRoute -DestinationPrefix '%s' -NextHop '%s' -PolicyStore ActiveStore -ErrorAction SilentlyContinue | Select-Object -First 1 InterfaceIndex,RouteMetric | ConvertTo-Json -Compress", cidr, nextHop)
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.Output()
	if err != nil {
		return 0, 0
	}
	var item struct {
		InterfaceIndex int `json:"InterfaceIndex"`
		RouteMetric    int `json:"RouteMetric"`
	}
	if json.Unmarshal(out, &item) != nil {
		return 0, 0
	}
	return item.RouteMetric, item.InterfaceIndex
}

func routeExists(rows []RouteRow, cidr, gateway string) bool {
	for _, row := range rows {
		if row.Cidr() == cidr && row.Gateway == gateway {
			return true
		}
	}
	return false
}

func cidrDestMask(cidr string) (string, string, error) {
	_, network, err := net.ParseCIDR(cidr)
	if err != nil || network.IP.To4() == nil {
		return "", "", fmt.Errorf("无效 IPv4 网段: %s", cidr)
	}
	mask := net.IP(network.Mask).String()
	return network.IP.String(), mask, nil
}

func interfaceIndexForIP(address string) int {
	want := net.ParseIP(address)
	if want == nil {
		return 0
	}
	interfaces, err := net.Interfaces()
	if err != nil {
		return 0
	}
	for _, iface := range interfaces {
		addresses, _ := iface.Addrs()
		for _, item := range addresses {
			ip, _, err := net.ParseCIDR(item.String())
			if err == nil && ip.Equal(want) {
				return iface.Index
			}
		}
	}
	return 0
}

// ReadRoutes 读当前系统路由表；route print -4 解析失败时回退 PowerShell Get-NetRoute。
func ReadRoutes() ([]RouteRow, []RouteRow, error) {
	cmd := exec.Command(routeExe, "print", "-4")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.Output()
	if err == nil {
		a, p, perr := ParseRoutePrint4(out)
		if perr == nil {
			return a, p, nil
		}
	}
	a, p, err := readRoutesPowerShell()
	return a, p, err
}

// ReadActiveRoutes 从 NetTCPIP 的 ActiveStore 读取路由。RouteMetric 是 route.exe
// 展示的“有效跃点”之外的实际路由跃点，适合与 route add METRIC 的值比较。
func ReadActiveRoutes() ([]RouteRow, error) {
	return readRoutesPowerShellStore("ActiveStore")
}

func readRoutesPowerShell() ([]RouteRow, []RouteRow, error) {
	active, err := readRoutesPowerShellStore("ActiveStore")
	if err != nil {
		return nil, nil, err
	}
	persistent, err := readRoutesPowerShellStore("PersistentStore")
	if err != nil {
		return nil, nil, err
	}
	return active, persistent, nil
}

func readRoutesPowerShellStore(store string) ([]RouteRow, error) {
	ps := fmt.Sprintf(`Get-NetRoute -AddressFamily IPv4 -PolicyStore %s | Select-Object DestinationPrefix,NextHop,InterfaceIndex,RouteMetric | ConvertTo-Json -Compress`, store)
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", ps)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("读取 %s 路由失败: %v", store, err)
	}
	rows := parseNetRouteJSON(out)
	// PersistentStore 可以合法为空；ActiveStore 空则意味着读取结果异常。
	if len(rows) == 0 && store == "ActiveStore" {
		return nil, fmt.Errorf("读取活动路由失败: 无数据")
	}
	return rows, nil
}
