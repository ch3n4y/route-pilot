//go:build windows

package routecmd

import (
	"bytes"
	"fmt"
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

// Add 添加持久路由：route -p add <dest> MASK <mask> <gw> [METRIC n] [IF idx]
func Add(gw, dest, mask string, metric, ifIndex int) error {
	args := []string{"-p", "add", dest, "MASK", mask, gw}
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

func readRoutesPowerShell() ([]RouteRow, []RouteRow, error) {
	ps := `Get-NetRoute -AddressFamily IPv4 | Select-Object DestinationPrefix,NextHop,InterfaceIndex,RouteMetric | ConvertTo-Json`
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", ps)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.Output()
	if err != nil {
		return nil, nil, fmt.Errorf("读取路由失败: %v", err)
	}
	rows := parseNetRouteJSON(out)
	if len(rows) == 0 {
		return nil, nil, fmt.Errorf("读取路由失败: 无数据")
	}
	return rows, nil, nil
}
