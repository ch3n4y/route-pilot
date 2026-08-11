package main

import (
	"embed"
	"io/fs"
	"log"
)

//go:embed all:web/dist
var webFS embed.FS

func staticFS() (fs.FS, error) {
	return fs.Sub(webFS, "web/dist")
}

// mustStatic 返回内嵌前端；缺失时记录错误并返回 nil（仅 API 可用）。
func mustStatic() fs.FS {
	sub, err := staticFS()
	if err != nil {
		log.Println("内嵌前端不可用: ", err)
		return nil
	}
	return sub
}
