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
			// 行内找 metric（从后往前第一个数字 token）
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
