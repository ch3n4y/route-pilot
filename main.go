package main

import (
	"flag"
	"log"

	"github.com/gin-gonic/gin"
)

var version = "0.1.0"

func main() {
	dev := flag.Bool("dev", false, "dev mode: serve from disk, allow vite CORS")
	flag.Parse()
	log.Printf("RouteManager %s (dev=%v)", version, *dev)

	r := gin.Default()
	r.GET("/api/health", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true, "version": version}) })
	if err := r.Run("0.0.0.0:8080"); err != nil {
		log.Fatal(err)
	}
}
