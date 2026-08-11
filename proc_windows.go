//go:build windows

package main

import (
	"os/exec"
)

// openBrowser 用默认浏览器打开 url。explorer.exe 经由已存在的 shell 以非提权方式打开，
// 避免从提权进程拉起提权浏览器。
func openBrowser(url string) {
	if err := exec.Command("explorer.exe", url).Start(); err == nil {
		return
	}
	_ = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
}
