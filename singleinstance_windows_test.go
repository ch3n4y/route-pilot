//go:build windows

package main

import "testing"

// TestSingleInstanceMutex 验证互斥量的"已存在"裁决：
// 首次获取应成功；同进程再次获取同名互斥量应判定为"已有实例"。
func TestSingleInstanceMutex(t *testing.T) {
	if !holdInstanceMutex() {
		t.Fatal("首次获取单实例互斥量应成功")
	}
	t.Cleanup(releaseInstanceMutex)

	if !anotherInstanceRunning() {
		t.Fatal("持有互斥量后，预检应能发现已有实例")
	}
	if holdInstanceMutex() {
		t.Fatal("再次获取同名互斥量应判定为已有实例")
	}
}
