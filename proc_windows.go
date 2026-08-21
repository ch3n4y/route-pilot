//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"golang.org/x/sys/windows/registry"
)

// openBrowser 用系统默认浏览器打开 url。首选 cmd /c start（ShellExecute 语义，
// 交给系统关联的默认浏览器，参考 gist.github.com/sevkin/9798d67b2cb9d07cb05f89f14ba682f8）；
// 失败（如未注册默认浏览器）时回退直接启动本机已安装的浏览器。
func openBrowser(url string) error {
	start := exec.Command("cmd", "/c", "start", "", url)
	start.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := start.Run(); err == nil {
		return nil
	}
	browser := findLocalBrowser()
	if browser == "" {
		return fmt.Errorf("未找到本地浏览器，请手动打开 %s", url)
	}
	return exec.Command(browser, url).Start()
}

func findLocalBrowser() string {
	names := []string{"msedge.exe", "chrome.exe", "firefox.exe"}
	for _, root := range []registry.Key{registry.CURRENT_USER, registry.LOCAL_MACHINE} {
		for _, name := range names {
			key, err := registry.OpenKey(root, `SOFTWARE\Microsoft\Windows\CurrentVersion\App Paths\`+name, registry.QUERY_VALUE)
			if err != nil {
				continue
			}
			path, _, valueErr := key.GetStringValue("")
			key.Close()
			if valueErr == nil && isFile(path) {
				return path
			}
		}
	}

	local := os.Getenv("LOCALAPPDATA")
	programFiles := os.Getenv("ProgramFiles")
	programFilesX86 := os.Getenv("ProgramFiles(x86)")
	candidates := []string{
		filepath.Join(programFilesX86, "Microsoft", "Edge", "Application", "msedge.exe"),
		filepath.Join(programFiles, "Microsoft", "Edge", "Application", "msedge.exe"),
		filepath.Join(local, "Microsoft", "Edge", "Application", "msedge.exe"),
		filepath.Join(programFiles, "Google", "Chrome", "Application", "chrome.exe"),
		filepath.Join(programFilesX86, "Google", "Chrome", "Application", "chrome.exe"),
		filepath.Join(local, "Google", "Chrome", "Application", "chrome.exe"),
		filepath.Join(programFiles, "Mozilla Firefox", "firefox.exe"),
		filepath.Join(programFilesX86, "Mozilla Firefox", "firefox.exe"),
	}
	for _, path := range candidates {
		if isFile(path) {
			return path
		}
	}
	return ""
}

func isFile(path string) bool {
	if path == "" || !filepath.IsAbs(path) {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
