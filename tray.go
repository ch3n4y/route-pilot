//go:build windows

package main

import (
	_ "embed"

	"github.com/getlantern/systray"
)

//go:embed icon.ico
var iconBytes []byte

// trayPort 由 main 赋值，供托盘"打开界面"使用。
var trayPort = "38254"

// runTray 启动系统托盘（阻塞直到退出），onQuit 在用户点"退出"时回调。
func runTray(onQuit func()) {
	systray.Run(func() {
		systray.SetIcon(iconBytes)
		systray.SetTitle("路由管理")
		systray.SetTooltip("跳板机路由管理")
		mOpen := systray.AddMenuItem("打开界面", "在浏览器中打开管理界面")
		mQuit := systray.AddMenuItem("退出", "停止服务并退出")
		go func() {
			for {
				select {
				case <-mOpen.ClickedCh:
					openBrowser("http://127.0.0.1:" + trayPort)
				case <-mQuit.ClickedCh:
					if onQuit != nil {
						onQuit()
					}
					systray.Quit()
					return
				}
			}
		}()
	}, func() {})
}
