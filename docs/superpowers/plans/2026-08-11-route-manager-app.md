# 路由管理 App（跳板机静态路由管理）Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 Windows 跳板机上构建一个单 exe 的 Web 路由管理工具：多对多管理「IP段→网关」静态路由，支持独占切换与批量切换，实时写入系统路由表。

**Architecture:** Go 1.26 + Gin 后端（SQLite 纯 Go 持久化），Vue 3 + Element Plus 前端构建产物经 `go:embed` 打进单二进制。后端内置路由同步引擎：期望态（DB 绑定关系）与系统实际路由表 diff 后自动增删，启动时全量重放。双击 exe → UAC 提权 → 监听局域网 + 登录密码 → 自动开浏览器。

**Tech Stack:** Go 1.26 / Gin / GORM / `github.com/glebarez/sqlite`（纯 Go，driver name `"sqlite"`）/ `golang.org/x/sys/windows` / `golang.org/x/crypto/bcrypt`；Vue 3 / Vite / Element Plus / Pinia / Vue Router / Axios。

## Global Constraints

- 仅 Windows 平台；路由写入/系统集成代码用 `//go:build windows` 隔离，纯逻辑（解析、CIDR）跨平台可测。
- 路由命令统一走 `C:\Windows\System32\route.exe`，`-p` 必须在 `add`/`delete` 前（`route -p add ...`）。`syscall.SysProcAttr{HideWindow: true}`。
- **解析器必须本地化无关**：本机 `route print -4` 是中文 GBK 输出，只按数字/位置解析，绝不匹配表头文字。解析失败回退 PowerShell `Get-NetRoute` JSON。
- 网关（下一跳）是**手动填写的局域网内其他设备 IP**，不是本机网卡默认网关。本机子网探测仅作表单候选建议。`route add` 不强制传 IF，让 Windows 自动选接口；网关不在任何本机子网时预警。
- SQLite 用 `github.com/glebarez/sqlite`（**不是** `gorm.io/driver/sqlite`）。单活动网关约束用部分唯一索引 `idx_bindings_one_active_per_segment`。
- 只删除本应用创建的路由（`applied_routes` 表追踪）；用户手工路由冲突时标记 CONFLICT，不自动删。
- 认证：bcrypt 密码（`settings.admin_password`）、32 字节随机 token（存 sha256 + 30 天过期）。`/api` 下除 `health`、`setup/*`、`login` 外均需 `Authorization: Bearer <token>`。
- 前端中文 UI；所有 API 返回 `{error: "..."}`（错误）或业务数据（成功）；成功写操作返回 204 或 200。
- 构建产物输出 `web/dist`，`npm run build` 必须先于 `go build`。
- 所有命令以管理员可读的中文/英文错误消息返回 UI。

---

### Task 1: 项目脚手架 + git 基线

**Files:**
- Create: `go.mod`、`.gitignore`、`internal/config/config.go`、`main.go`（最小版：解析 flag、启动 Gin、/api/health）

**Interfaces:**
- Produces: `config.Load(dev bool) *config.AppConfig`，字段 `Host string`、`Port int`、`DataDir string`、`Dev bool`。

- [ ] **Step 1: 初始化 go module**

```bash
cd d:/code/Go/Router
go mod init route-manager
go get github.com/gin-gonic/gin github.com/glebarez/sqlite gorm.io/gorm golang.org/x/crypto/bcrypt golang.org/x/sys/windows
git add go.mod go.sum && git commit -m "chore: init module"
```

- [ ] **Step 2: 写 `.gitignore`**

```gitignore
node_modules/
web/dist/
*.exe
*.db
*.log
dist/
```
提交。

- [ ] **Step 3: 写 `internal/config/config.go`**

```go
package config

import (
	"os"
	"path/filepath"
)

type AppConfig struct {
	Host    string
	Port    int
	DataDir string
	Dev     bool
}

func Load(dev bool) *AppConfig {
	dir := filepath.Dir(os.Args[0])
	if _, err := os.Stat(dir); err != nil {
		dir, _ = os.Getwd()
	}
	// exe 目录不可写时退回 LOCALAPPDATA
	dataDir := dir
	f, err := os.Create(filepath.Join(dir, ".write_test"))
	if err != nil {
		dataDir = filepath.Join(os.Getenv("LOCALAPPDATA"), "RouteManager")
		os.MkdirAll(dataDir, 0o755)
	} else {
		f.Close()
		os.Remove(filepath.Join(dir, ".write_test"))
	}
	return &AppConfig{Host: "0.0.0.0", Port: 8080, DataDir: dataDir, Dev: dev}
}
```

- [ ] **Step 4: 写最小 `main.go`**

```go
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
	r.Run("0.0.0.0:8080")
}
```

- [ ] **Step 5: 构建并验证**

```bash
go build . && go vet ./...
```
Expected: 编译通过。`go run .` 后 `curl http://localhost:8080/api/health` 返回 `{"ok":true,...}`。

- [ ] **Step 6: 提交**

```bash
git add -A && git commit -m "feat: scaffold gin server with health endpoint"
```

---

### Task 2: 数据模型 + SQLite 层

**Files:**
- Create: `internal/models/models.go`、`internal/db/db.go`

**Interfaces:**
- Consumes: `config.Load` → `*config.AppConfig`
- Produces:
  - `models.Segment`、`models.Gateway`、`models.Binding`、`models.Setting`、`models.AuthToken`、`models.AppliedRoute`（GORM struct）
  - `db.Open(cfg *config.AppConfig) (*gorm.DB, error)` — 打开/建库、AutoMigrate、建部分唯一索引、seed `settings`（port/host/sync_on_change）
  - `db.GetSetting(db, key, fallback) string`、`db.SetSetting(db, key, val) error`

- [ ] **Step 1: 写 `internal/models/models.go`**

```go
package models

import "time"

type Segment struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	Name        string    `gorm:"not null" json:"name"`
	Cidr        string    `gorm:"not null;uniqueIndex" json:"cidr"` // 规范化网络地址 10.0.0.0/8
	Netmask     string    `gorm:"not null" json:"netmask"`
	Description string    `gorm:"default:''" json:"description"`
	Enabled     bool      `gorm:"default:true" json:"enabled"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Gateway struct {
	ID          uint   `gorm:"primaryKey" json:"id"`
	Name        string `gorm:"not null" json:"name"`
	GatewayIP   string `gorm:"not null" json:"gateway_ip"`
	Interface   string `gorm:"default:''" json:"interface"`
	IfIndex     int    `json:"ifindex"`
	Metric      int    `gorm:"default:1" json:"metric"`
	Description string `gorm:"default:''" json:"description"`
	Enabled     bool   `gorm:"default:true" json:"enabled"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Binding struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	SegmentID uint      `gorm:"not null;index:idx_seg_gw,unique" json:"segment_id"`
	GatewayID uint      `gorm:"not null;index:idx_seg_gw,unique" json:"gateway_id"`
	IsActive  bool      `gorm:"default:false" json:"is_active"`
	Position  int       `gorm:"default:0" json:"position"`
	Enabled   bool      `gorm:"default:true" json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Setting struct {
	Key   string `gorm:"primaryKey" json:"key"`
	Value string `json:"value"`
}

type AuthToken struct {
	TokenHash string    `gorm:"primaryKey" json:"-"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

type AppliedRoute struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	SegmentID  uint      `json:"segment_id"`
	Cidr       string    `gorm:"not null" json:"cidr"`
	GatewayIP  string    `gorm:"not null" json:"gateway_ip"`
	Metric     int       `gorm:"default:1" json:"metric"`
	IfIndex    int       `json:"ifindex"`
	Status     string    `gorm:"default:'OK'" json:"status"` // OK|MISSING|CONFLICT|ERROR
	LastError  string    `gorm:"default:''" json:"last_error"`
	LastSyncAt time.Time `json:"last_sync_at"`
}
```

- [ ] **Step 2: 写 `internal/db/db.go`**

```go
package db

import (
	"path/filepath"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"route-manager/internal/config"
	"route-manager/internal/models"
)

func Open(cfg *config.AppConfig) (*gorm.DB, error) {
	path := filepath.Join(cfg.DataDir, "RouteManager.db")
	gdb, err := gorm.Open(sqlite.Open(path+"?_pragma=foreign_keys(1)"), &gorm.Config{})
	if err != nil {
		return nil, err
	}
	if err := gdb.AutoMigrate(&models.Segment{}, &models.Gateway{}, &models.Binding{},
		&models.Setting{}, &models.AuthToken{}, &models.AppliedRoute{}); err != nil {
		return nil, err
	}
	// 单活动网关约束（SQLite 部分唯一索引）
	if err := gdb.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_bindings_one_active_per_segment
		ON bindings (segment_id) WHERE is_active = 1`).Error; err != nil {
		return nil, err
	}
	// seed 默认 settings
	for k, v := range map[string]string{"host": cfg.Host, "port": "8080", "sync_on_change": "1"} {
		if GetSetting(gdb, k, "") == "" {
			_ = SetSetting(gdb, k, v)
		}
	}
	return gdb, nil
}

func GetSetting(gdb *gorm.DB, key, fallback string) string {
	var s models.Setting
	if err := gdb.First(&s, "key = ?", key).Error; err != nil {
		return fallback
	}
	return s.Value
}

func SetSetting(gdb *gorm.DB, key, val string) error {
	var s models.Setting
	if err := gdb.First(&s, "key = ?", key).Error; err != nil {
		return gdb.Create(&models.Setting{Key: key, Value: val}).Error
	}
	return gdb.Model(&s).Update("value", val).Error
}
```

- [ ] **Step 3: 验证编译**

```bash
go build ./... && go vet ./...
```
Expected: PASS（gorm 依赖已装）。

- [ ] **Step 4: 提交**

```bash
git add -A && git commit -m "feat: models and sqlite layer with one-active-per-segment index"
```

---

### Task 3: 认证模块（bcrypt + token）

**Files:**
- Create: `internal/auth/auth.go`

**Interfaces:**
- Consumes: `db.GetSetting/SetSetting`、`models.AuthToken`、`*gorm.DB`
- Produces:
  - `auth.HashPassword(pw string) (string, error)`
  - `auth.VerifyPassword(hash, pw string) bool`
  - `auth.IsSetup(gdb *gorm.DB) bool`
  - `auth.CreateToken(gdb *gorm.DB) (string, time.Time, error)` — 返回明文 token + 过期时间
  - `auth.Login(gdb *gorm.DB, pw string) (string, time.Time, bool)`
  - `auth.RevokeToken(gdb *gorm.DB, token string) error`
  - `auth.Middleware(gdb *gorm.DB) gin.HandlerFunc`

- [ ] **Step 1: 写 `internal/auth/auth.go`**

```go
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"route-manager/internal/models"
)

const tokenTTL = 30 * 24 * time.Hour

func HashPassword(pw string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	return string(b), err
}

func VerifyPassword(hash, pw string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(pw)) == nil
}

func IsSetup(gdb *gorm.DB) bool {
	return dbSetting(gdb, "admin_password", "") != ""
}

func dbSetting(gdb *gorm.DB, key, fb string) string {
	var s models.Setting
	if err := gdb.First(&s, "key = ?", key).Error; err != nil {
		return fb
	}
	return s.Value
}

func CreateToken(gdb *gorm.DB) (string, time.Time, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", time.Time{}, err
	}
	token := hex.EncodeToString(raw)
	exp := time.Now().Add(tokenTTL)
	hash := sha256Hex(token)
	return token, exp, gdb.Create(&models.AuthToken{TokenHash: hash, ExpiresAt: exp}).Error
}

func Login(gdb *gorm.DB, pw string) (string, time.Time, bool) {
	hash := dbSetting(gdb, "admin_password", "")
	if hash == "" || !VerifyPassword(hash, pw) {
		return "", time.Time{}, false
	}
	token, exp, err := CreateToken(gdb)
	return token, exp, err == nil
}

func RevokeToken(gdb *gorm.DB, token string) error {
	return gdb.Delete(&models.AuthToken{}, "token_hash = ?", sha256Hex(token)).Error
}

func Middleware(gdb *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.GetHeader("Authorization")
		token := strings.TrimPrefix(h, "Bearer ")
		if token == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
			return
		}
		var t models.AuthToken
		if err := gdb.First(&t, "token_hash = ? AND expires_at > ?", sha256Hex(token), time.Now()).Error; err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "登录已过期"})
			return
		}
		c.Set("token", token)
		c.Next()
	}
}

func sha256Hex(s string) string {
	b := sha256.Sum256([]byte(s))
	return hex.EncodeToString(b[:])
}
```

- [ ] **Step 2: 验证编译**

```bash
go build ./... && go vet ./...
```
Expected: PASS。

- [ ] **Step 3: 提交**

```bash
git add -A && git commit -m "feat: bcrypt + bearer token auth"
```

---

### Task 4: routecmd 解析器（本地化无关）— TDD

**Files:**
- Create: `internal/routecmd/parse.go`、`internal/routecmd/parse_test.go`

**Interfaces:**
- Produces:
  - `routecmd.RouteRow{Dest, Mask, Gateway, Interface string; Metric int; Persistent bool}`
  - `routecmd.ParseRoutePrint4(out []byte) (active []RouteRow, persistent []RouteRow, err error)` — 纯函数，无 shell 依赖

**算法（关键）**：按行切分 → 以"全等号"行为块分隔符 → 路由行 = 行首两个空白 token 均为点分 IPv4 → `dest, mask, gw(若第3个 token 是点分 IP，否则 on-link), iface, metric`。活动路由行 5 个四元组，持久路由行 4 个（gw metric），两者 token[0..2] 都是 dest mask gw。含路由行的**最后一个块**为持久、前一块为活动。解析结果为空且失败时由调用方回退 `Get-NetRoute`。

- [ ] **Step 1: 写失败测试 `internal/routecmd/parse_test.go`**

```go
package routecmd

import "testing"

const sample = `===========================================================================
接口列表
 14...00 15 5d 0c 9a 08 ......Intel(R) I211 Gigabit Network Connection
  1...........................Software Loopback Interface 1
===========================================================================
IPv4 路由表
===========================================================================
活动路由:
        网络目标          网络掩码          网关       接口   跃点数
          0.0.0.0          0.0.0.0     192.168.1.1    192.168.1.9     25
       10.99.0.0      255.255.0.0     192.168.1.2    192.168.1.9     11
===========================================================================
持久路由:
  网络目标          网络掩码          网关       跃点数
     10.99.0.0      255.255.0.0     192.168.1.2       1
===========================================================================
`

func TestParseRoutePrint4(t *testing.T) {
	active, persistent, err := ParseRoutePrint4([]byte(sample))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(active) != 2 {
		t.Fatalf("active routes = %d, want 2", len(active))
	}
	if active[1].Dest != "10.99.0.0" || active[1].Mask != "255.255.0.0" ||
		active[1].Gateway != "192.168.1.2" || active[1].Metric != 11 {
		t.Fatalf("active[1] = %+v", active[1])
	}
	if len(persistent) != 1 || persistent[0].Gateway != "192.168.1.2" {
		t.Fatalf("persistent = %+v", persistent)
	}
}
```

- [ ] **Step 2: 运行确认失败**

```bash
cd d:/code/Go/Router && go test ./internal/routecmd/ -run TestParseRoutePrint4 -v
```
Expected: FAIL（parse.go 不存在）。

- [ ] **Step 3: 写 `internal/routecmd/parse.go`**

```go
package routecmd

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
)

type RouteRow struct {
	Dest       string
	Mask       string
	Gateway    string
	Interface  string
	Metric     int
	Persistent bool
}

func isIPv4(s string) bool {
	parts := strings.Split(s, ".")
	if len(parts) != 4 {
		return false
	}
	for _, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 || n > 255 {
			return false
		}
	}
	return true
}

// ParseRoutePrint4 解析 route print -4 输出。本地化无关：只信任数字与位置，不匹配表头文字。
// 以全等号行分块；含路由行的最后一个块为持久路由，其前一含路由行的块为活动路由。
func ParseRoutePrint4(out []byte) (active []RouteRow, persistent []RouteRow, err error) {
	text := string(bytes.ToValidUTF8(out, nil))
	lines := strings.Split(text, "\n")
	var blocks [][]string
	var cur []string
	for _, ln := range lines {
		t := strings.TrimSpace(ln)
		if t == "" {
			continue
		}
		allEq := len(t) >= 4 && strings.Trim(t, "=") == ""
		if allEq {
			if len(cur) > 0 {
				blocks = append(blocks, cur)
				cur = nil
			}
			continue
		}
		cur = append(cur, ln)
	}
	if len(cur) > 0 {
		blocks = append(blocks, cur)
	}

	parseBlock := func(lines []string) []RouteRow {
		var rows []RouteRow
		for _, ln := range lines {
			f := strings.Fields(ln)
			if len(f) < 3 || !isIPv4(f[0]) || !isIPv4(f[1]) {
				continue
			}
			r := RouteRow{Dest: f[0], Mask: f[1]}
			if len(f) >= 3 && isIPv4(f[2]) {
				r.Gateway = f[2]
			} else {
				r.Gateway = "on-link"
			}
			// 行内找 metric（最后一个数字 token）；非点分 token 是本地化标记
			metric := 0
			for i := len(f) - 1; i >= 2; i-- {
				if n, e := strconv.Atoi(f[i]); e == nil {
					metric = n
					break
				}
			}
			r.Metric = metric
			rows = append(rows, r)
		}
		return rows
	}

	var withRows [][]RouteRow
	for _, b := range blocks {
		rows := parseBlock(b)
		if len(rows) > 0 {
			withRows = append(withRows, rows)
		}
	}
	switch len(withRows) {
	case 0:
		return nil, nil, fmt.Errorf("no route rows found")
	case 1:
		return withRows[0], nil, nil
	default:
		persistent = withRows[len(withRows)-1]
		active = withRows[len(withRows)-2]
		return active, persistent, nil
	}
}
```

- [ ] **Step 4: 运行确认通过**

```bash
go test ./internal/routecmd/ -run TestParseRoutePrint4 -v
```
Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add -A && git commit -m "feat: locale-independent route print -4 parser"
```

---

### Task 5: routecmd 路由执行 + netutil CIDR — TDD

**Files:**
- Create: `internal/routecmd/route_windows.go`、`internal/netutil/cidr.go`、`internal/netutil/cidr_test.go`

**Interfaces:**
- Produces:
  - `routecmd.Add(gw, dest, mask string, metric, ifIndex int) error`
  - `routecmd.Delete(gw, dest, mask string) error`
  - `routecmd.ReadRoutes() ([]RouteRow, []RouteRow, error)` — 运行 `route print -4` 并解析，失败时回退 `Get-NetRoute`（同文件内 `//go:build windows`）
  - `netutil.CanonicalCIDR(s string) (network string, netmask string, err error)` — 规范化 `10.1.2.3/8` → `("10.0.0.0/8","255.0.0.0")`
  - `netutil.Overlaps(a, b string) bool` — 两个 CIDR 是否重叠

- [ ] **Step 1: 写失败测试 `internal/netutil/cidr_test.go`**

```go
package netutil

import "testing"

func TestCanonicalCIDR(t *testing.T) {
	n, m, err := CanonicalCIDR("10.1.2.3/8")
	if err != nil || n != "10.0.0.0/8" || m != "255.0.0.0" {
		t.Fatalf("got %q %q err=%v", n, m, err)
	}
	n, _, err = CanonicalCIDR("10.0.0.0/32")
	if err != nil || n != "10.0.0.0/32" {
		t.Fatalf("/32 got %q err=%v", n, err)
	}
	n, m, err = CanonicalCIDR("0.0.0.0/0")
	if err != nil || n != "0.0.0.0/0" || m != "0.0.0.0" {
		t.Fatalf("/0 got %q %q err=%v", n, m, err)
	}
	if _, _, err = CanonicalCIDR("fe80::1/64"); err == nil {
		t.Fatal("ipv6 should be rejected")
	}
}

func TestOverlaps(t *testing.T) {
	if !Overlaps("10.0.0.0/8", "10.5.0.0/16") {
		t.Fatal("10.0.0.0/8 should overlap 10.5.0.0/16")
	}
	if Overlaps("10.0.0.0/8", "172.16.0.0/12") {
		t.Fatal("should not overlap")
	}
}
```

- [ ] **Step 2: 运行确认失败**

```bash
go test ./internal/netutil/ -run 'TestCanonicalCIDR|TestOverlaps' -v
```
Expected: FAIL。

- [ ] **Step 3: 写 `internal/netutil/cidr.go`**

```go
package netutil

import (
	"fmt"
	"net"
)

// CanonicalCIDR 规范化用户输入：10.1.2.3/8 -> 网络地址 10.0.0.0/8 + 掩码 255.0.0.0。
// 仅 IPv4；拒绝主机地址之外的格式错误。
func CanonicalCIDR(s string) (network string, netmask string, err error) {
	ip, ipnet, err := net.ParseCIDR(s)
	if err != nil {
		return "", "", fmt.Errorf("CIDR 格式错误: %s", s)
	}
	if ip.To4() == nil {
		return "", "", fmt.Errorf("仅支持 IPv4: %s", s)
	}
	n := ipnet.IP.To4()
	m := net.IP(ipnet.Mask)
	return fmt.Sprintf("%d.%d.%d.%d/%d", n[0], n[1], n[2], n[3], ones(ipnet.Mask)),
		fmt.Sprintf("%d.%d.%d.%d", m[0], m[1], m[2], m[3]), nil
}

func ones(mask net.IPMask) int {
	n := 0
	for _, b := range mask {
		for ; b > 0; b <<= 1 {
			n++
		}
	}
	return n
}

// Overlaps 判断两个 CIDR 是否重叠。
func Overlaps(a, b string) bool {
	_, na, err1 := net.ParseCIDR(a)
	_, nb, err2 := net.ParseCIDR(b)
	if err1 != nil || err2 != nil {
		return false
	}
	return na.Contains(nb.IP) || nb.Contains(na.IP)
}
```

- [ ] **Step 4: 运行确认通过**

```bash
go test ./internal/netutil/ -run 'TestCanonicalCIDR|TestOverlaps' -v
```
Expected: PASS。

- [ ] **Step 5: 写 `internal/routecmd/route_windows.go`（`//go:build windows`）**

```go
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

// ReadRoutes 读当前路由表；route print -4 解析失败时回退 PowerShell Get-NetRoute。
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
```

- [ ] **Step 6: 加 PowerShell 回退 `readRoutesPowerShell`**（同文件）

```go
func readRoutesPowerShell() ([]RouteRow, []RouteRow, error) {
	ps := `Get-NetRoute -AddressFamily IPv4 | Select-Object DestinationPrefix,NextHop,InterfaceIndex,RouteMetric | ConvertTo-Json`
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", ps)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.Output()
	if err != nil {
		return nil, nil, fmt.Errorf("读取路由失败: %v", err)
	}
	// 简单解析 [{DestinationPrefix,NextHop,InterfaceIndex,RouteMetric}]
	rows := parseNetRouteJSON(out)
	if len(rows) == 0 {
		return nil, nil, fmt.Errorf("读取路由失败: 无数据")
	}
	return rows, nil, nil
}
```

`parseNetRouteJSON` 为纯函数放 `parse.go`（解析 JSON 数组 → RouteRow，Persistent 全 false）。**Step 7 中实现。**

- [ ] **Step 7: 在 `parse.go` 追加 `parseNetRouteJSON`**

```go
func parseNetRouteJSON(b []byte) []RouteRow {
	type item struct {
		DestinationPrefix string `json:"DestinationPrefix"`
		NextHop           string `json:"NextHop"`
		InterfaceIndex    int    `json:"InterfaceIndex"`
		RouteMetric       int    `json:"RouteMetric"`
	}
	var list []item
	if err := json.Unmarshal(b, &list); err != nil {
		return nil
	}
	var rows []RouteRow
	for _, it := range list {
		dest, mask, err := splitPrefix(it.DestinationPrefix) // "10.0.0.0/8" -> "10.0.0.0","255.0.0.0"
		if err != nil {
			continue
		}
		rows = append(rows, RouteRow{Dest: dest, Mask: mask, Gateway: it.NextHop, Interface: fmt.Sprintf("%d", it.InterfaceIndex), Metric: it.RouteMetric})
	}
	return rows
}

func splitPrefix(p string) (string, string, error) {
	parts := strings.SplitN(p, "/", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("bad prefix %s", p)
	}
	var plen int
	if _, err := fmt.Sscanf(parts[1], "%d", &plen); err != nil {
		return "", "", err
	}
	mask := net.IP(net.CIDRMask(plen, 32))
	return parts[0], mask.String(), nil
}
```

- [ ] **Step 8: 编译验证**

```bash
go build ./... && go vet ./...
```
Expected: PASS。

- [ ] **Step 9: 提交**

```bash
git add -A && git commit -m "feat: route.exe add/delete and CIDR utils"
```

---

### Task 6: netutil 探测 + 提权模块

**Files:**
- Create: `internal/netutil/detect.go`、`internal/elevate/elevate_windows.go`

**Interfaces:**
- Produces:
  - `netutil.LocalInterfaces() ([]InterfaceInfo, error)`，`InterfaceInfo{Index int; Name string; IPs []string; DefaultGateway string}` — 用 Go `net.Interfaces()` + `net.InterfaceAddrs()`，加 `DefaultGateway`（PowerShell `Get-NetIPConfiguration`，失败留空）
  - `netutil.GatewayReachableOnInterface(gw string) (ifIndex int, ok bool)` — 网关 IP 落在哪个本机子网
  - `elevate.IsElevated() bool`
  - `elevate.RelaunchElevated() error` — `ShellExecute(0, "runas", ...)`

- [ ] **Step 1: 写 `internal/netutil/detect.go`**

```go
package netutil

import (
	"net"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

type InterfaceInfo struct {
	Index           int      `json:"index"`
	Name            string   `json:"name"`
	IPs             []string `json:"ips"`
	DefaultGateway  string   `json:"default_gateway"`
}

// LocalInterfaces 枚举本机网卡（仅 IPv4 地址），用于网关表单候选建议（非数据来源）。
func LocalInterfaces() ([]InterfaceInfo, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	gw := defaultGatewayMap()
	var out []InterfaceInfo
	for _, it := range ifaces {
		if it.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, _ := it.Addrs()
		info := InterfaceInfo{Index: it.Index, Name: it.Name}
		for _, a := range addrs {
			ipn, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			if ipn.IP.To4() != nil {
				info.IPs = append(info.IPs, ipn.IP.String())
			}
		}
		if len(info.IPs) > 0 {
			info.DefaultGateway = gw[it.Index]
			out = append(out, info)
		}
	}
	return out, nil
}

// GatewayReachableOnInterface 返回网关 IP 所在的本地接口 index（0 = 不在任何本机子网）。
func GatewayReachableOnInterface(gw string) (int, bool) {
	ip := net.ParseIP(gw)
	if ip == nil || ip.To4() == nil {
		return 0, false
	}
	ifaces, err := net.Interfaces()
	if err != nil {
		return 0, false
	}
	for _, it := range ifaces {
		addrs, _ := it.Addrs()
		for _, a := range addrs {
			ipn, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			if ipn.Contains(ip) {
				return it.Index, true
			}
		}
	}
	return 0, false
}

func defaultGatewayMap() map[int]string {
	m := map[int]string{}
	ps := `Get-NetIPConfiguration | Where-Object {$_.IPv4DefaultGateway} | ForEach-Object { [PSCustomObject]@{ Index=$_.InterfaceIndex; GW=$_.IPv4DefaultGateway.NextHop } } | ConvertTo-Json`
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", ps)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.Output()
	if err != nil {
		return m
	}
	// 解析 [{Index, GW}]
	fields := strings.Fields(string(out))
	for i, f := range fields {
		if strings.Contains(f, "Index") {
			// 简单容错解析：匹配 "Index": 3 或 {"Index":3
			for _, seg := range fields {
				if strings.HasPrefix(seg, `"Index"`) {
					_ = seg
				}
			}
		}
		_ = i
	}
	// 更稳的方式：逐行 JSON 手工提取
	extractGatewayJSON(m, string(out))
	return m
}
```

`extractGatewayJSON` 手工解析 `"Index": n` 与 `"GW": "ip"` 配对（避免引入编码库）。**Step 2 实现。**

- [ ] **Step 2: 在 `detect.go` 追加 `extractGatewayJSON`**

```go
func extractGatewayJSON(m map[int]string, s string) {
	// 找到所有 "Index":<num> 和 "GW":"<ip>"，按对象顺序配对。
	toks := strings.Split(s, "{")
	curIdx := 0
	for _, obj := range toks {
		idx, gw := 0, ""
		for _, line := range strings.Split(obj, ",") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, `"Index"`) {
				parts := strings.Split(line, ":")
				if len(parts) == 2 {
					if n, err := strconv.Atoi(strings.Trim(strings.TrimSpace(parts[1]), `"`)); err == nil {
						idx = n
					}
				}
			}
			if strings.HasPrefix(line, `"GW"`) {
				parts := strings.SplitN(line, ":", 2)
				if len(parts) == 2 {
					gw = strings.Trim(strings.TrimSpace(parts[1]), `"`)
				}
			}
		}
		if idx != 0 && gw != "" {
			m[idx] = gw
		}
		_ = curIdx
	}
}
```

- [ ] **Step 3: 写 `internal/elevate/elevate_windows.go`（`//go:build windows`）**

```go
//go:build windows

package elevate

import (
	"os"

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
	return windows.ShellExecute(0, verb, exePtr, args, cwd, 1) // 1 = SW_NORMAL
}
```

需要 `import "strings"`。

- [ ] **Step 4: 编译验证**

```bash
go build ./... && go vet ./...
```
Expected: PASS（Windows 下可编译）。

- [ ] **Step 5: 提交**

```bash
git add -A && git commit -m "feat: interface probing and UAC elevation"
```

---

### Task 7: 同步引擎（核心）— TDD

**Files:**
- Create: `internal/sync/engine.go`

**Interfaces:**
- Consumes: `routecmd.ReadRoutes/Add/Delete`、`netutil.GatewayReachableOnInterface`、`models.*`
- Produces:
  - `sync.Engine` struct：`New(gdb *gorm.DB) *Engine`；`Reconcile() Result`；`RequestSync()`（异步，去抖）；`Status() Result`；`ForceReplace(segmentID uint) error`（CONFLICT 时强制用应用网关替换）
  - `sync.Result`：`Syncing bool; LastSyncAt time.Time; Desired []DesiredEntry; Entries []Entry; Summary map[string]int`
  - `DesiredEntry{SegmentID uint; SegmentName, Cidr, GatewayIP string; Metric, IfIndex int}`
  - `Entry{SegmentID uint; SegmentName, Cidr, GatewayIP string; Status, Error string}`

**同步算法：**
1. 期望态：`JOIN segments + bindings(is_active=1, enabled=1) + gateways(enabled=1)` where `segments.enabled=1`。
2. 实际态：`routecmd.ReadRoutes()` → 按 `Dest|Mask` 为 key 的 `map[key]gateway`。
3. 分类每期望项：实际存在且网关一致=OK；存在网关不同=CONFLICT；不存在=MISSING。
4. 对 MISSING：`Add(gw, dest, mask, metric, ifIndex)`（ifIndex 由 `GatewayReachableOnInterface` 得出，返回 0 则不传），成功 upsert `applied_routes`；失败记 ERROR。
5. 对 CONFLICT（仅当 applied_routes 里有该 cidr 的记录，即我们创建过）：不自动处理，标记 CONFLICT，等 ForceReplace 或用户手动。
6. 清理：`applied_routes` 中 `(cidr,gw)` 不在期望态 → `Delete`，删除记录。

- [ ] **Step 1: 写核心结构（纯 diff 逻辑拆成可测函数 `classify`）**

```go
package sync

import (
	"sync"
	"time"

	"gorm.io/gorm"

	"route-manager/internal/models"
	"route-manager/internal/routecmd"
	"route-manager/internal/netutil"
)

type DesiredEntry struct {
	SegmentID   uint   `json:"segment_id"`
	SegmentName string `json:"segment_name"`
	Cidr        string `json:"cidr"`
	GatewayIP   string `json:"gateway_ip"`
	Metric      int    `json:"metric"`
	IfIndex     int    `json:"ifindex"`
}

type Entry struct {
	SegmentID   uint   `json:"segment_id"`
	SegmentName string `json:"segment_name"`
	Cidr        string `json:"cidr"`
	GatewayIP   string `json:"gateway_ip"`
	Status      string `json:"status"` // OK|MISSING|CONFLICT|ERROR
	Error       string `json:"error"`
}

type Result struct {
	Syncing    bool          `json:"syncing"`
	LastSyncAt time.Time     `json:"last_sync_at"`
	Desired    []DesiredEntry `json:"desired"`
	Entries    []Entry       `json:"entries"`
	Summary    map[string]int `json:"summary"`
}

type Engine struct {
	db     *gorm.DB
	mu     sync.Mutex
	result Result
	syncing bool
}
```

**接口函数签名（Step 2+ 实现）：** `New`, `desired() ([]DesiredEntry, error)`, `actualMap() (map[string]string, error)`, `classify(des []DesiredEntry, actual map[string]string) []Entry`, `Reconcile() Result`, `Status() Result`, `RequestSync()`, `ForceReplace(segmentID uint) error`。

- [ ] **Step 2: 写 `desired` + `actualMap` + `classify`（纯逻辑）**

```go
func desired(db *gorm.DB) ([]DesiredEntry, error) {
	var rows []DesiredEntry
	err := db.Table("segments").
		Select("segments.id AS segment_id, segments.name AS segment_name, segments.cidr, gateways.gateway_ip, gateways.metric, gateways.ifindex").
		Joins("JOIN bindings ON bindings.segment_id = segments.id AND bindings.is_active = 1 AND bindings.enabled = 1").
		Joins("JOIN gateways ON gateways.id = bindings.gateway_id AND gateways.enabled = 1").
		Where("segments.enabled = 1").
		Scan(&rows).Error
	return rows, err
}

func actualMap() (map[string]string, error) {
	active, _, err := routecmd.ReadRoutes()
	if err != nil {
		return nil, err
	}
	m := map[string]string{}
	for _, r := range active {
		if r.Gateway == "on-link" {
			continue
		}
		m[r.Dest+"|"+r.Mask] = r.Gateway
	}
	return m, nil
}

func classify(des []DesiredEntry, actual map[string]string) []Entry {
	var out []Entry
	for _, d := range des {
		e := Entry{SegmentID: d.SegmentID, SegmentName: d.SegmentName, Cidr: d.Cidr, GatewayIP: d.GatewayIP}
		if gw, ok := actual[d.Cidr]; ok {
			if gw == d.GatewayIP {
				e.Status = "OK"
			} else {
				e.Status = "CONFLICT"
				e.Error = "系统中已存在网关 " + gw + " 的同网段路由"
			}
		} else {
			e.Status = "MISSING"
		}
		out = append(out, e)
	}
	return out
}
```

注意 `actualMap` 以 `Cidr`（`10.0.0.0/8`）为 key —— 需把 `Dest|Mask` 转回 CIDR。在 `classify` 调用前将 actual 的 key 统一为 CIDR：在 `actualMap` 中用 `routecmd` 提供 `MaskToCIDR` 或在 parse.go 加 `func (r RouteRow) Cidr() string`。

- [ ] **Step 3: 在 `parse.go` 追加 `RouteRow.Cidr()`**

```go
import "net"

// Cidr 由 dest+mask 还原 CIDR 字符串（如 10.99.0.0 + 255.255.0.0 -> 10.99.0.0/16）
func (r RouteRow) Cidr() string {
	ip := net.ParseIP(r.Dest).To4()
	m := net.IPv4Mask(
		ipByte(r.Mask, 0), ipByte(r.Mask, 1), ipByte(r.Mask, 2), ipByte(r.Mask, 3),
	)
	ones, _ := m.Size()
	return ip.String() + "/" + itoa(ones)
}
```

并提供辅助 `ipByte`、`itoa`（用 `strconv.Itoa`）。然后在 `actualMap` 改用 `m[r.Cidr()] = r.Gateway`。

- [ ] **Step 4: 写失败测试 `internal/sync/classify_test.go`**

```go
package sync

import "testing"

func TestClassify(t *testing.T) {
	des := []DesiredEntry{
		{SegmentID: 1, Cidr: "10.0.0.0/8", GatewayIP: "192.168.1.2"},
		{SegmentID: 2, Cidr: "172.16.0.0/16", GatewayIP: "192.168.1.3"},
	}
	actual := map[string]string{"10.0.0.0/8": "192.168.1.2", "172.16.0.0/16": "192.168.1.99"}
	got := classify(des, actual)
	if got[0].Status != "OK" {
		t.Fatalf("want OK got %+v", got[0])
	}
	if got[1].Status != "CONFLICT" {
		t.Fatalf("want CONFLICT got %+v", got[1])
	}
}
```

- [ ] **Step 5: 运行确认失败 → 实现 → 通过**

```bash
go test ./internal/sync/ -run TestClassify -v
```
Expected: FAIL（`classify` 未导出前先用小写，测试同包可访问）→ 实现后 PASS。

- [ ] **Step 6: 实现 `Reconcile` / `Status` / `RequestSync` / `ForceReplace`**

```go
func (e *Engine) Status() Result {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.result
}

func (e *Engine) RequestSync() {
	if e.syncing {
		return
	}
	go func() {
		e.mu.Lock()
		e.syncing = true
		e.mu.Unlock()
		res := e.Reconcile()
		e.mu.Lock()
		e.result = res
		e.syncing = false
		e.mu.Unlock()
	}()
}

func (e *Engine) Reconcile() Result {
	res := Result{Syncing: true, LastSyncAt: time.Now(), Summary: map[string]int{"ok": 0, "missing": 0, "conflict": 0, "error": 0}}
	des, err := desired(e.db)
	if err != nil {
		res.Summary["error"]++
		return res
	}
	actual, err := actualMap()
	if err != nil {
		// 读失败：标记全部未知，仍返回条目
		actual = map[string]string{}
	}
	res.Desired = des
	res.Entries = classify(des, actual)
	for _, en := range res.Entries {
		switch en.Status {
		case "OK":
			res.Summary["ok"]++
		case "MISSING":
			res.Summary["missing"]++
		case "CONFLICT":
			res.Summary["conflict"]++
		case "ERROR":
			res.Summary["error"]++
		}
	}
	// apply MISSING（顺序）
	for i := range res.Entries {
		if res.Entries[i].Status != "MISSING" {
			continue
		}
		d := des[i]
		ifIdx, ok := netutil.GatewayReachableOnInterface(d.GatewayIP)
		if !ok && d.Cidr == "0.0.0.0/0" {
			ifIdx = 0 // 默认路由允许自动选
		}
		if err := routecmd.Add(d.GatewayIP, cidrDest(d.Cidr), cidrMask(d.Cidr), d.Metric, ifIdx); err != nil {
			res.Entries[i].Status = "ERROR"
			res.Entries[i].Error = err.Error()
			res.Summary["error"]++
			res.Summary["missing"]--
			continue
		}
		res.Entries[i].Status = "OK"
		res.Summary["missing"]--
		res.Summary["ok"]++
		e.upsertApplied(d, ifIdx)
	}
	e.cleanup(des)
	res.Syncing = false
	return res
}
```

提供辅助：`cidrDest(cidr) string`、`cidrMask(cidr) string`（`net.ParseCIDR` 拆解）、`upsertApplied(d DesiredEntry, ifIdx int)`、`cleanup(des []DesiredEntry) error`（遍历 `applied_routes`，不在期望态则 `routecmd.Delete` 并删记录）。

- [ ] **Step 7: 编译 + 测试通过**

```bash
go build ./... && go vet ./... && go test ./internal/sync/ -v
```
Expected: PASS。

- [ ] **Step 8: 提交**

```bash
git add -A && git commit -m "feat: route sync engine (desired/actual/apply)"
```

---

### Task 8: 服务端路由 + 全部 handlers

**Files:**
- Create: `internal/server/respond.go`、`internal/server/router.go`、`internal/server/handlers_auth.go`、`internal/server/handlers_segments.go`、`internal/server/handlers_gateways.go`、`internal/server/handlers_bindings.go`、`internal/server/handlers_routes.go`、`internal/server/handlers_settings.go`

**Interfaces:**
- Produces: `server.New(gdb *gorm.DB, eng *sync.Engine, cfg *config.AppConfig, ver string) *gin.Engine` — 挂 `/api/*`（auth 中间件 + 各 handler）+ 静态/SPA（Task 11 处理 embed 时再挂；此处先 `/api`）。main.go 调用 `r.Run(cfg.Host + ":" + port)`。

**Handler 职责（所有写操作成功后调用 `eng.RequestSync()`；切换/批量切换/手动同步同步执行返回结果）：**

- [ ] **Step 1: `respond.go`**

```go
package server

import "github.com/gin-gonic/gin"

func ok(c *gin.Context, data any)      { c.JSON(200, data) }
func created(c *gin.Context, data any) { c.JSON(201, data) }
func noContent(c *gin.Context)         { c.Status(204) }
func fail(c *gin.Context, status int, msg string) { c.JSON(status, gin.H{"error": msg}) }
```

- [ ] **Step 2: `handlers_auth.go`**

```go
package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"route-manager/internal/auth"
)

func (s *Server) hSetupStatus(c *gin.Context) { ok(c, gin.H{"needs_setup": !auth.IsSetup(s.db)}) }

func (s *Server) hSetup(c *gin.Context) {
	var body struct{ Password string `json:"password"` }
	if err := c.ShouldBindJSON(&body); err != nil || body.Password == "" {
		fail(c, 400, "密码不能为空"); return
	}
	if auth.IsSetup(s.db) { fail(c, 409, "已初始化"); return }
	hash, err := auth.HashPassword(body.Password)
	if err != nil { fail(c, 500, err.Error()); return }
	if err := s.db.Create(&models.Setting{Key: "admin_password", Value: hash}).Error; err != nil {
		fail(c, 500, err.Error()); return
	}
	token, exp, err := auth.CreateToken(s.db)
	if err != nil { fail(c, 500, err.Error()); return }
	created(c, gin.H{"token": token, "expires_at": exp})
}

func (s *Server) hLogin(c *gin.Context) {
	var body struct{ Password string `json:"password"` }
	if err := c.ShouldBindJSON(&body); err != nil {
		fail(c, 400, "参数错误"); return
	}
	token, exp, ok := auth.Login(s.db, body.Password)
	if !ok { fail(c, 401, "密码错误"); return }
	ok(c, gin.H{"token": token, "expires_at": exp})
}

func (s *Server) hLogout(c *gin.Context) {
	t := c.GetString("token")
	_ = auth.RevokeToken(s.db, t)
	noContent(c)
}

func (s *Server) hMe(c *gin.Context) {
	ok(c, gin.H{"password_set": auth.IsSetup(s.db), "elevated": s.elevated})
}
```

定义 `Server` struct：`Server{db *gorm.DB; eng *sync.Engine; cfg *config.AppConfig; version string; elevated bool}`，构造 `s := &Server{...}`。

- [ ] **Step 3: `handlers_segments.go` — CRUD + switch + batch-switch**

```go
// segmentWithMeta 前端需要的附加字段：活动网关 id + 候选绑定列表（含网关名）
type segmentWithMeta struct {
	models.Segment
	ActiveGatewayID uint        `json:"active_gateway_id"`
	Bindings        []bindingVM `json:"bindings"`
}

type bindingVM struct {
	ID          uint   `json:"id"`
	GatewayID   uint   `json:"gateway_id"`
	GatewayName string `json:"gateway_name"`
	IsActive    bool   `json:"is_active"`
	Enabled     bool   `json:"enabled"`
}

func (s *Server) hSegments(c *gin.Context) {
	var segs []models.Segment
	s.db.Order("id").Find(&segs)
	var binds []models.Binding
	s.db.Order("position").Find(&binds)
	gwName := map[uint]string{}
	var gws []models.Gateway
	s.db.Find(&gws)
	for _, g := range gws {
		gwName[g.ID] = g.Name
	}
	items := make([]segmentWithMeta, 0, len(segs))
	for _, seg := range segs {
		vm := segmentWithMeta{Segment: seg, Bindings: []bindingVM{}}
		for _, b := range binds {
			if b.SegmentID != seg.ID {
				continue
			}
			if b.IsActive {
				vm.ActiveGatewayID = b.GatewayID
			}
			vm.Bindings = append(vm.Bindings, bindingVM{ID: b.ID, GatewayID: b.GatewayID, GatewayName: gwName[b.GatewayID], IsActive: b.IsActive, Enabled: b.Enabled})
		}
		items = append(items, vm)
	}
	ok(c, gin.H{"items": items})
}

func (s *Server) hCreateSegment(c *gin.Context) {
	var body struct {
		Name, Cidr, Description string
		Enabled                 *bool
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		fail(c, 400, "参数错误"); return
	}
	network, mask, err := netutil.CanonicalCIDR(body.Cidr)
	if err != nil { fail(c, 422, err.Error()); return }
	// 重叠检测
	var count int64
	s.db.Model(&models.Segment{}).Where("enabled = 1").Count(&count)
	var all []models.Segment
	s.db.Find(&all)
	for _, ex := range all {
		if netutil.Overlaps(ex.Cidr, network) {
			fail(c, 422, "与现有网段 "+ex.Cidr+" 重叠"); return
		}
	}
	enabled := true
	if body.Enabled != nil { enabled = *body.Enabled }
	seg := models.Segment{Name: body.Name, Cidr: network, Netmask: mask, Description: body.Description, Enabled: enabled}
	if err := s.db.Create(&seg).Error; err != nil {
		fail(c, 422, "创建失败: "+err.Error()); return
	}
	s.eng.RequestSync()
	created(c, gin.H{"item": seg})
}
```

`hUpdateSegment`（PUT，按 id 更新 name/cidr(重新规范化+重叠校验)/description/enabled）、`hDeleteSegment`（DELETE，删除后级联绑定，`eng.RequestSync()`）。**每个 handler 写操作后调 `eng.RequestSync()`。**

`hSwitchSegment` / `hBatchSwitch` 逻辑见 Step 4。

- [ ] **Step 4: 切换 + 批量切换（核心业务）**

```go
// 事务内：先清该段活动，再 upsert 目标网关绑定并置为活动。
func (s *Server) activateBinding(segmentID, gatewayID uint) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.Binding{}).Where("segment_id = ?", segmentID).Update("is_active", false).Error; err != nil {
			return err
		}
		var b models.Binding
		err := tx.Where("segment_id = ? AND gateway_id = ?", segmentID, gatewayID).First(&b).Error
		if err == gorm.ErrRecordNotFound {
			b = models.Binding{SegmentID: segmentID, GatewayID: gatewayID, IsActive: true, Enabled: true}
			return tx.Create(&b).Error
		}
		if err != nil {
			return err
		}
		return tx.Model(&b).Update("is_active", true).Error
	})
}

func (s *Server) hSwitchSegment(c *gin.Context) {
	id := atou(c.Param("id"))
	var body struct{ GatewayID uint `json:"gateway_id"` }
	if err := c.ShouldBindJSON(&body); err != nil { fail(c, 400, "参数错误"); return }
	if err := s.activateBinding(id, body.GatewayID); err != nil { fail(c, 409, err.Error()); return }
	res := s.eng.Reconcile() // 同步执行
	ok(c, gin.H{"ok": true, "status": res})
}

func (s *Server) hBatchSwitch(c *gin.Context) {
	var body struct {
		SegmentIDs []uint `json:"segment_ids"`
		GatewayID  uint   `json:"gateway_id"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || len(body.SegmentIDs) == 0 {
		fail(c, 400, "参数错误"); return
	}
	results := []gin.H{}
	for _, sid := range body.SegmentIDs {
		if err := s.activateBinding(sid, body.GatewayID); err != nil {
			results = append(results, gin.H{"segment_id": sid, "ok": false, "error": err.Error()})
		} else {
			results = append(results, gin.H{"segment_id": sid, "ok": true})
		}
	}
	res := s.eng.Reconcile()
	ok(c, gin.H{"results": results, "status": res})
}
```

- [ ] **Step 5: `handlers_gateways.go` — CRUD + `GET /api/network/interfaces`**

```go
func (s *Server) hGateways(c *gin.Context) {
	var items []models.Gateway
	s.db.Order("id").Find(&items)
	// 附带 used_by：每个网关被哪些网段使用
	ok(c, gin.H{"items": items})
}

func (s *Server) hCreateGateway(c *gin.Context) {
	var body struct {
		Name, GatewayIP, Interface, Description string
		IfIndex, Metric                          int
		Enabled                                  *bool
	}
	if err := c.ShouldBindJSON(&body); err != nil { fail(c, 400, "参数错误"); return }
	if net.ParseIP(body.GatewayIP) == nil || net.ParseIP(body.GatewayIP).To4() == nil {
		fail(c, 422, "网关必须是合法 IPv4"); return
	}
	enabled := true
	if body.Enabled != nil { enabled = *body.Enabled }
	gw := models.Gateway{Name: body.Name, GatewayIP: body.GatewayIP, Interface: body.Interface,
		IfIndex: body.IfIndex, Metric: body.Metric, Description: body.Description, Enabled: enabled}
	if err := s.db.Create(&gw).Error; err != nil { fail(c, 422, err.Error()); return }
	created(c, gin.H{"item": gw})
}

func (s *Server) hNetworkInterfaces(c *gin.Context) {
	ifaces, err := netutil.LocalInterfaces()
	if err != nil { fail(c, 500, err.Error()); return }
	ok(c, gin.H{"interfaces": ifaces})
}
```

`hUpdateGateway` / `hDeleteGateway`（删除网关级联绑定 + `eng.RequestSync()`）。

- [ ] **Step 6: `handlers_bindings.go` + `handlers_routes.go` + `handlers_settings.go`**

bindings：`hBindings`（可选 `?segment_id=&gateway_id=` 过滤）、`hCreateBinding`（UNIQUE(seg,gw) 冲突返回 409）、`hUpdateBinding`（enabled/position/is_active）、`hDeleteBinding`、`hSetActive`（调 `activateBinding`，同步 Reconcile 返回）。

routes：
```go
func (s *Server) hRouteStatus(c *gin.Context)  { ok(c, s.eng.Status()) }
func (s *Server) hRouteSync(c *gin.Context)    { ok(c, s.eng.Reconcile()) }
func (s *Server) hRouteActual(c *gin.Context) {
	active, persistent, err := routecmd.ReadRoutes()
	if err != nil { fail(c, 500, err.Error()); return }
	ok(c, gin.H{"active": active, "persistent": persistent})
}
```

settings：
```go
func (s *Server) hSettings(c *gin.Context) {
	ok(c, gin.H{
		"port": db.GetSetting(s.db, "port", "8080"),
		"host": db.GetSetting(s.db, "host", s.cfg.Host),
		"data_dir": s.cfg.DataDir,
		"version": s.version,
		"elevated": s.elevated,
		"sync_on_change": db.GetSetting(s.db, "sync_on_change", "1"),
	})
}
func (s *Server) hChangePassword(c *gin.Context) {
	var body struct{ OldPassword, NewPassword string `json:"old_password"` }
	if err := c.ShouldBindJSON(&body); err != nil || body.NewPassword == "" {
		fail(c, 400, "参数错误"); return
	}
	cur := db.GetSetting(s.db, "admin_password", "")
	if !auth.VerifyPassword(cur, body.OldPassword) { fail(c, 401, "旧密码错误"); return }
	hash, err := auth.HashPassword(body.NewPassword)
	if err != nil { fail(c, 500, err.Error()); return }
	if err := db.SetSetting(s.db, "admin_password", hash); err != nil { fail(c, 500, err.Error()); return }
	noContent(c)
}
```

- [ ] **Step 7: `router.go` 组装**

```go
package server

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"route-manager/internal/auth"
	"route-manager/internal/config"
	"route-manager/internal/sync"
)

type Server struct {
	db       *gorm.DB
	eng      *sync.Engine
	cfg      *config.AppConfig
	version  string
	elevated bool
}

func New(gdb *gorm.DB, eng *sync.Engine, cfg *config.AppConfig, ver string, elevated bool, static fs.FS) *gin.Engine {
	s := &Server{db: gdb, eng: eng, cfg: cfg, version: ver, elevated: elevated}
	r := gin.Default()
	api := r.Group("/api")
	api.GET("/health", func(c *gin.Context) { ok(c, gin.H{"ok": true, "version": ver}) })
	api.GET("/setup/status", s.hSetupStatus)
	api.POST("/setup", s.hSetup)
	api.POST("/login", s.hLogin)

	authed := api.Group("")
	authed.Use(auth.Middleware(gdb))
	authed.GET("/me", s.hMe)
	authed.POST("/logout", s.hLogout)
	authed.GET("/segments", s.hSegments)
	authed.POST("/segments", s.hCreateSegment)
	authed.PUT("/segments/:id", s.hUpdateSegment)
	authed.DELETE("/segments/:id", s.hDeleteSegment)
	authed.POST("/segments/:id/switch", s.hSwitchSegment)
	authed.POST("/segments/batch-switch", s.hBatchSwitch)
	authed.GET("/gateways", s.hGateways)
	authed.POST("/gateways", s.hCreateGateway)
	authed.PUT("/gateways/:id", s.hUpdateGateway)
	authed.DELETE("/gateways/:id", s.hDeleteGateway)
	authed.GET("/network/interfaces", s.hNetworkInterfaces)
	authed.GET("/bindings", s.hBindings)
	authed.POST("/bindings", s.hCreateBinding)
	authed.PUT("/bindings/:id", s.hUpdateBinding)
	authed.DELETE("/bindings/:id", s.hDeleteBinding)
	authed.POST("/bindings/set-active", s.hSetActive)
	authed.GET("/routes/status", s.hRouteStatus)
	authed.POST("/routes/sync", s.hRouteSync)
	authed.GET("/routes/actual", s.hRouteActual)
	authed.GET("/settings", s.hSettings)
	authed.PUT("/settings/password", s.hChangePassword)

	// 静态 + SPA 兜底（dev 模式 static 传 nil，前端走 vite）
	if static != nil {
		r.StaticFS("/", http.FS(static))
		r.NoRoute(func(c *gin.Context) {
			if strings.HasPrefix(c.Request.URL.Path, "/api") {
				c.JSON(404, gin.H{"error": "not found"})
				return
			}
			b, _ := fs.ReadFile(static, "index.html")
			c.Data(200, "text/html; charset=utf-8", b)
		})
	}
	return r
}
```
imports 需含 `"io/fs"`、`"strings"`。

- [ ] **Step 8: 编译**

```bash
go build ./... && go vet ./...
```
Expected: PASS（`atou` 辅助用 `strconv.Atoi`，放 respond.go）。

- [ ] **Step 9: 提交**

```bash
git add -A && git commit -m "feat: full REST API with auth, switch and batch-switch"
```

---

### Task 9: 前端脚手架（Vite + Vue3 + Element Plus）

**Files:**
- Create: `web/package.json`、`web/vite.config.js`、`web/index.html`、`web/src/main.js`、`web/src/App.vue`、`web/src/router/index.js`、`web/src/api/index.js`、`web/src/stores/auth.js`

**Interfaces:**
- Produces: `npm run dev`（:5173，proxy `/api`→:8080）；`npm run build`（→ `web/dist`）
- Produces: `src/api/index.js` 导出 `http`（axios 实例，`baseURL:'/api'`，请求拦截器加 Bearer，401 清 token 跳 /login）；stores：`auth.js`（token/password_set/needs_setup + login/setup/logout/me）

- [ ] **Step 1: `web/package.json`**

```json
{
  "name": "route-manager-web",
  "private": true,
  "version": "0.1.0",
  "scripts": { "dev": "vite", "build": "vite build" },
  "dependencies": {
    "vue": "^3.5.0", "element-plus": "^2.9.0", "@element-plus/icons-vue": "^2.3.1",
    "pinia": "^2.2.0", "vue-router": "^4.4.0", "axios": "^1.7.0"
  },
  "devDependencies": { "vite": "^5.4.0", "@vitejs/plugin-vue": "^5.1.0" }
}
```

- [ ] **Step 2: `web/vite.config.js`**

```js
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
export default defineConfig({
  plugins: [vue()],
  base: './',
  server: { port: 5173, proxy: { '/api': 'http://127.0.0.1:8080' } },
  build: { outDir: 'dist', emptyOutDir: true },
})
```

- [ ] **Step 3: `web/index.html` + `src/main.js` + `src/App.vue`**

`index.html`：标准 Vue3 模板，`<div id="app">`，加载 `/src/main.js`，`<html lang="zh-CN">`。
`main.js`：
```js
import { createApp } from 'vue'
import { createPinia } from 'pinia'
import ElementPlus from 'element-plus'
import 'element-plus/dist/index.css'
import zhCn from 'element-plus/es/locale/lang/zh-cn'
import App from './App.vue'
import router from './router'
import './api' // 初始化拦截器副作用可省略，改在 api/index.js 内导入
const app = createApp(App)
app.use(createPinia()).use(router).use(ElementPlus, { locale: zhCn })
app.mount('#app')
```

`App.vue`：`<router-view/>`（布局含侧边栏的改造放 Task 12）。

- [ ] **Step 4: `src/router/index.js`**

```js
import { createRouter, createWebHistory } from 'vue-router'
const routes = [
  { path: '/login', component: () => import('../views/Login.vue') },
  { path: '/setup', component: () => import('../views/Setup.vue') },
  { path: '/', component: () => import('../views/Segments.vue') },
  { path: '/segments', component: () => import('../views/Segments.vue') },
  { path: '/gateways', component: () => import('../views/Gateways.vue') },
  { path: '/matrix', component: () => import('../views/Matrix.vue') },
  { path: '/routes', component: () => import('../views/Routes.vue') },
  { path: '/settings', component: () => import('../views/Settings.vue') },
]
export default createRouter({ history: createWebHistory(), routes })
```

- [ ] **Step 5: `src/api/index.js` + `src/stores/auth.js`**

```js
import axios from 'axios'
import router from '../router'
const http = axios.create({ baseURL: '/api' })
http.interceptors.request.use(cfg => {
  const t = localStorage.getItem('token')
  if (t) cfg.headers.Authorization = `Bearer ${t}`
  return cfg
})
http.interceptors.response.use(r => r.data, err => {
  if (err.response?.status === 401) {
    localStorage.removeItem('token')
    router.push('/login')
  }
  return Promise.reject(err.response?.data || { error: '网络错误' })
})
export default http
```

`auth.js`：
```js
import { defineStore } from 'pinia'
import http from '../api'
export const useAuthStore = defineStore('auth', {
  state: () => ({ token: localStorage.getItem('token') || '', needsSetup: false, elevated: false, passwordSet: false }),
  actions: {
    async init() {
      const st = await http.get('/setup/status')
      this.needsSetup = st.needs_setup
      if (this.token) { const me = await http.get('/me'); this.elevated = me.elevated; this.passwordSet = me.password_set }
    },
    async login(pw) { const r = await http.post('/login', { password: pw }); this.token = r.token; localStorage.setItem('token', r.token) },
    async setup(pw) { const r = await http.post('/setup', { password: pw }); this.token = r.token; localStorage.setItem('token', r.token) },
    async logout() { try { await http.post('/logout') } catch {} ; this.token = ''; localStorage.removeItem('token') },
  },
})
```

- [ ] **Step 6: 建最小占位 views（Login.vue 可先空壳）避免路由报错，验证构建**

```bash
cd web && npm install && npm run build
```
Expected: `web/dist` 生成。提交。

- [ ] **Step 7: 提交**

```bash
git add -A && git commit -m "feat: vue3+element-plus frontend scaffold with auth store"
```

---

### Task 10: 登录/安装/网段页

**Files:**
- Create: `web/src/views/Login.vue`、`web/src/views/Setup.vue`、`web/src/views/Segments.vue`

**Interface：** Login/Setup 调 `auth` store；Segments 页调 `GET/POST/PUT/DELETE /segments`、`POST /segments/:id/switch`、`POST /segments/batch-switch`，支持多选批量切换（el-table `type=selection`）。

- [ ] **Step 1: `Login.vue`** — 密码输入 + `auth.login()` + `router.push('/')`。
- [ ] **Step 2: `Setup.vue`** — 首次设置密码 + `auth.setup()`。
- [ ] **Step 3: `Segments.vue`** — 完整 CRUD：

```vue
<template>
  <div>
    <el-button type="primary" @click="dialog = true">新增网段</el-button>
    <el-button type="warning" :disabled="!selection.length" @click="openBatchSwitch">
      批量切换网关 ({{ selection.length }})</el-button>
    <el-table :data="items" @selection-change="s => selection = s">
      <el-table-column type="selection" width="40" />
      <el-table-column prop="name" label="名称" />
      <el-table-column prop="cidr" label="网段" />
      <el-table-column prop="netmask" label="掩码" width="110" />
      <el-table-column label="活动网关" width="220">
        <template #default="{ row }">
          <el-select v-model="row.active_gateway_id" placeholder="切换网关" @change="gw => switchGateway(row, gw)">
            <el-option v-for="b in row.bindings" :key="b.id" :value="b.gateway_id" :label="b.gateway_name || ('网关#'+b.gateway_id)" />
          </el-select>
        </template>
      </el-table-column>
      <el-table-column prop="description" label="备注" />
      <el-table-column label="操作" width="140">
        <template #default="{ row }">
          <el-button size="small" @click="edit(row)">编辑</el-button>
          <el-button size="small" type="danger" @click="del(row)">删除</el-button>
        </template>
      </el-table-column>
    </el-table>
    <!-- 新增/编辑对话框 + 批量切换对话框（选网关 -> POST batch-switch） -->
  </div>
</template>
```

`GET /segments` 返回的 item 中，`active_gateway_id` 与 `bindings` 由后端补齐（Task 8 的 `hSegments` 需补 preload：`s.db.Preload(...)` 或用一次查询组装）。**注意：hSegments 需返回 `active_gateway_id` 与 `bindings[].gateway_name`，否则前端切换下拉无数据。** 在 `hSegments` 里补充：查所有 binding+gateway 组装。

- [ ] **Step 4: 实现段内切换 `switchGateway(row, gw)`** → `POST /segments/:id/switch` → 刷新列表。
- [ ] **Step 5: 实现 `openBatchSwitch`** → 对话框选网关 → `POST /segments/batch-switch {segment_ids, gateway_id}` → 展示 results → 刷新。

**依赖处理：** `hSegments` 的组装逻辑放后端（Task 8 Step 3 中说明），前端不重复计算。

- [ ] **Step 6: 提交**

```bash
git add -A && git commit -m "feat: login/setup/segments pages with batch switch"
```

---

### Task 11: 网关页 + 绑定矩阵页

**Files:**
- Create: `web/src/views/Gateways.vue`、`web/src/views/Matrix.vue`

- [ ] **Step 1: `Gateways.vue`** — CRUD + "本机子网建议"按钮（`GET /network/interfaces`，弹出可选项把 IP 填入网关 IP 输入框）+ 显示该网关服务的网段数。
- [ ] **Step 2: `Matrix.vue`** — 绑定矩阵：

```vue
<template>
  <el-table :data="segments" border>
    <el-table-column prop="name" label="IP 段" min-width="160" fixed />
    <el-table-column v-for="g in gateways" :key="g.id" :label="g.name" align="center">
      <template #default="{ row }">
        <el-tooltip v-if="cell(row, g.id)" :content="cell(row, g.id) === 'active' ? '活动网关' : '候选网关'">
          <el-radio :model-value="row.active_gateway_id === g.id"
                    @change="v => setActive(row, g.id)">{{ cell(row, g.id) === 'active' ? '●' : '○' }}</el-radio>
        </el-tooltip>
        <el-tooltip content="点击添加为候选">
          <el-button v-else size="small" circle text @click="addCandidate(row, g.id)">＋</el-button>
        </el-tooltip>
      </template>
    </el-table-column>
  </el-table>
</template>
```

- [ ] **Step 3: 数据**：`cell(row, gwId)` 读 `row.bindings`；`setActive` → `POST /bindings/set-active`；`addCandidate` → `POST /bindings {segment_id, gateway_id}`。切页或操作后刷新 segments+gateways。
- [ ] **Step 4: 提交**

```bash
git add -A && git commit -m "feat: gateways page and binding matrix"
```

---

### Task 12: 路由状态页 + 设置页 + 布局

**Files:**
- Create: `web/src/views/Routes.vue`、`web/src/views/Settings.vue`、`web/src/views/NotFound.vue`
- Modify: `web/src/App.vue`（侧边栏布局 + 提权横幅 + 同步状态）

- [ ] **Step 1: `Routes.vue`** — `GET /routes/status` 每 2s 轮询（`syncing` 时）；展示 summary 徽章 + entries 表（状态色：OK 绿 / MISSING 橙 / CONFLICT 红 / ERROR 红）+ "手动同步"按钮（`POST /routes/sync`）+ `GET /routes/actual` 只读系统路由表（活动 + 持久两个折叠面板）。
- [ ] **Step 2: `Settings.vue`** — 展示 port/host/data_dir/version/elevated/sync_on_change；改密码表单（`PUT /settings/password`）。
- [ ] **Step 3: `App.vue`** — el-container 布局：侧边栏（网段/网关/绑定矩阵/路由状态/设置）、顶部栏（提权横幅 `!elevated` → 显示"当前只读，请以管理员重启" + 重启按钮）、`router-view`。路由守卫：未登录且需登录 → `/login`；`needs_setup` → `/setup`。

```js
// 在 router/index.js 加 beforeEach
router.beforeEach(async (to) => {
  const a = useAuthStore()
  await a.init()
  if (to.path === '/login' || to.path === '/setup') return true
  if (!a.token) return '/login'
  if (a.needsSetup && to.path !== '/setup') return '/setup'
  return true
})
```

- [ ] **Step 4: 构建**

```bash
cd web && npm run build
```
Expected: `web/dist` 生成无报错。提交。

```bash
git add -A && git commit -m "feat: routes/settings pages and app layout"
```

---

### Task 13: embed + 自动开浏览器 + main.go 收尾 + build.ps1

**Files:**
- Create: `embed.go`、`build.ps1`
- Modify: `main.go`、`internal/server/router.go`（挂静态/SPA）

- [ ] **Step 1: `embed.go`**

```go
package main

import (
	"embed"
	"io/fs"
)

//go:embed web/dist
var webFS embed.FS

func staticFS() (fs.FS, error) { return fs.Sub(webFS, "web/dist") }
```

- [ ] **Step 2: 静态/SPA 由 router.go 处理（已在 Task 8 实现）** — main 只需把 embed FS 传给 `server.New(..., static)`；dev 模式传 nil 并给 `server.New` 加 dev CORS：`corsMiddleware` 允许 `Origin: http://localhost:5173`（用 gin 中间件设置 `Access-Control-Allow-Origin`）。`embed.go` 在 main 包，`server` 包不 import main，靠参数注入避免循环依赖。

- [ ] **Step 3: main.go 完整逻辑**

```go
func main() {
	dev := flag.Bool("dev", false, "dev mode")
	noElevate := flag.Bool("no-elevate", false, "skip elevation (read-only)")
	flag.Parse()
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	if !*dev && !*noElevate && !elevate.IsElevated() {
		if err := elevate.RelaunchElevated(); err != nil {
			log.Println("提权失败，进入只读模式:", err)
		} else {
			return // 原实例退出，等提权实例
		}
	}
	elevated := *noElevate || elevate.IsElevated()

	cfg := config.Load(*dev)
	ensureLog(cfg) // 简单写 RouteManager.log（见下）
	gdb, err := db.Open(cfg)
	if err != nil { log.Fatal(err) }
	eng := sync.New(gdb)
	eng.Reconcile() // 启动全量重放

	var static fs.FS
	if !*dev { static, _ = staticFS() }
	r := server.New(gdb, eng, cfg, version, elevated, static)
	addr := cfg.Host + ":" + db.GetSetting(gdb, "port", "8080")
	go func() {
		time.Sleep(800 * time.Millisecond)
		openBrowser("http://127.0.0.1:" + db.GetSetting(gdb, "port", "8080"))
	}()
	if err := r.Run(addr); err != nil { log.Fatal(err) }
}
```

`openBrowser`（Windows）：`exec.Command("explorer.exe", url).Start()`，失败回退 `rundll32 url.dll,FileProtocolHandler <url>`。
`ensureLog`：将 `log.SetOutput` 指向 `RouteManager.log`（带 `--console` 时保留 stdout）。

- [ ] **Step 4: `build.ps1`**

```powershell
$ErrorActionPreference = "Stop"
Push-Location web
npm install
npm run build
Pop-Location
$env:CGO_ENABLED = "0"
go build -trimpath -ldflags "-s -w -H=windowsgui -X main.version=1.0.0" -o RouteManager.exe .
Write-Host "Build OK: RouteManager.exe"
```

- [ ] **Step 5: 验证构建 + 运行**

```bash
go build ./... && go vet ./... && pwsh -File build.ps1
```
Expected: 生成 `RouteManager.exe`。双击（或 `.\RouteManager.exe` 控制台模式 `.\RouteManager.exe --console`）→ UAC → 浏览器打开。

- [ ] **Step 6: 提交**

```bash
git add -A && git commit -m "feat: embed frontend, auto-open browser, release build"
```

---

### Task 14: 端到端验证 + 修复

**Files:**
- Modify: 各文件（按验证结果修复）

- [ ] **Step 1: 按以下清单在真实 Windows（管理员）验证**，逐条打勾，失败项记录并修复：
  1. 双击 exe → UAC → 浏览器开 `http://localhost:8080`；局域网机器访问 `http://<ip>:8080`（防火墙放行）。
  2. 首次设置密码 → 登录。
  3. 手动建网关 GW-LAN(192.168.1.2)、GW-VPN；"本机子网建议"列候选。
  4. 建网段 `10.99.0.0/16` → 回显 `10.99.0.0/16` + `255.255.0.0`；重复/重叠有 422 告警。
  5. 矩阵点选活动 → UI OK；`route print -4 | findstr 10.99.0.0` 活动+持久均有。
  6. 切换 GW-VPN → route print 网关变化。
  7. 3 网段批量切 → 全部生效；DB `SELECT segment_id,COUNT(*) FROM bindings WHERE is_active=1 GROUP BY segment_id` 全为 1。
  8. 删除 → route print 无残留。
  9. 网关填不可达 IP → UI ERROR + stderr，其他网段不受影响。
  10. `--no-elevate` 或取消 UAC → 只读可用，写操作提示需管理员。
  11. 重启 → 路由恢复（-p + 启动重放）。
- [ ] **Step 2: 跑全部 Go 测试**

```bash
go test ./...
```
Expected: 全 PASS。

- [ ] **Step 3: 最终提交**

```bash
git add -A && git commit -m "test: e2e verification fixes"
```

---

## Self-Review 备注

- 覆盖全部需求：多对多绑定（bindings 表 + 矩阵页）、独占切换（部分唯一索引 + activateBinding 事务）、批量切换（batch-switch API + 多选 UI）、CRUD（segments/gateways handlers）、网关手动填写（GatewayIP 手填 + 子网建议）、登录（auth）、单 exe 双击（embed + build.ps1 + 提权）、开机重放（启动 Reconcile）。
- `hSegments` 必须返回 `active_gateway_id` 与 `bindings`（含 gateway_name），否则前端切换下拉/矩阵无数据——Task 10 Step 5 已标注为后端补全。
- 类型一致性：`Engine.Reconcile()` 返回 `Result`；`RequestSync`/`Status` 均操作同一 `result`。`activateBinding` 被 switch/batch-switch/set-active 复用。
