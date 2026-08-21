//go:build windows

package main

import (
	"log"

	"golang.org/x/sys/windows"
)

// 单实例互斥量与"打开界面"通知事件。
// 名字固定（不取 exe 文件名），保证 路由管理.exe / router.exe 部署名下都互相排斥。
// Local\ 命名空间为会话级：提权前后（同一用户会话）可见，互斥有效。
const (
	singleInstanceMutexName = `Local\RouteManager.SingleInstance`
	openUIEventName         = `Local\RouteManager.OpenUI`
)

var instanceMutex windows.Handle

// anotherInstanceRunning 尝试打开单实例互斥量；能打开说明已有实例在运行。
// 找不到（ERROR_FILE_NOT_FOUND）或其他错误都视为"未运行"——最终裁决由提权后的
// holdInstanceMutex 的 CreateMutex 完成（该调用原子返回"已存在"）。
func anotherInstanceRunning() bool {
	h, err := windows.OpenMutex(windows.SYNCHRONIZE, false, windows.StringToUTF16Ptr(singleInstanceMutexName))
	if err != nil {
		return false
	}
	windows.CloseHandle(h)
	return true
}

// holdInstanceMutex 创建并持有单实例互斥量。仅在最终运行实例（提权后/降级只读模式）调用。
// 若已被其它进程先创建（并发首启竞态、或 OpenMutex 预检漏判），返回 false，调用方应退出。
func holdInstanceMutex() bool {
	h, err := windows.CreateMutex(nil, false, windows.StringToUTF16Ptr(singleInstanceMutexName))
	if err == windows.ERROR_ALREADY_EXISTS {
		windows.CloseHandle(h)
		return false
	}
	if err != nil {
		// 创建失败（非已存在）：无法保证互斥，保守放行避免误杀，但记录警告
		log.Println("警告: 创建单实例互斥量失败: ", err)
		return true
	}
	instanceMutex = h
	return true
}

// releaseInstanceMutex 退出时释放互斥量句柄。
func releaseInstanceMutex() {
	if instanceMutex != 0 {
		windows.CloseHandle(instanceMutex)
		instanceMutex = 0
	}
}

// notifyOpenUI 通知已运行实例打开界面（设置命名事件），随后本进程退出。
// 事件尚未创建时静默忽略——首实例启动本身会自动打开浏览器，不会漏。
func notifyOpenUI() {
	ev, err := windows.OpenEvent(windows.EVENT_MODIFY_STATE, false, windows.StringToUTF16Ptr(openUIEventName))
	if err != nil {
		return
	}
	windows.SetEvent(ev)
	windows.CloseHandle(ev)
}

// watchOpenUI 等待"打开界面"事件；收到信号则打开浏览器（重复启动时的 UX 补全）。
// 自动复位事件：一次 SetEvent 对应一次打开。
func watchOpenUI() {
	ev, err := windows.CreateEvent(nil, 0, 0, windows.StringToUTF16Ptr(openUIEventName))
	if err != nil && err != windows.ERROR_ALREADY_EXISTS {
		return
	}
	defer windows.CloseHandle(ev)
	for {
		if s, err := windows.WaitForSingleObject(ev, windows.INFINITE); err != nil || s != windows.WAIT_OBJECT_0 {
			return
		}
		openBrowser("http://127.0.0.1:" + trayPort)
	}
}
