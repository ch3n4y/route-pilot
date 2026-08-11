//go:build windows

package elevate

import (
	"os"
	"strings"

	"golang.org/x/sys/windows"
)

// IsElevated 当前进程是否已提权。
func IsElevated() bool {
	return windows.GetCurrentProcessToken().IsElevated()
}

// RelaunchElevated 以管理员身份重启动当前进程（触发 UAC 弹窗）。
func RelaunchElevated() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	verb, _ := windows.UTF16PtrFromString("runas")
	exePtr, _ := windows.UTF16PtrFromString(exe)
	args, _ := windows.UTF16PtrFromString(strings.Join(os.Args[1:], " "))
	cwd, _ := windows.UTF16PtrFromString("")
	// SW_NORMAL = 1
	return windows.ShellExecute(0, verb, exePtr, args, cwd, 1)
}
