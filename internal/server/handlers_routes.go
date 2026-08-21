package server

import (
	"github.com/gin-gonic/gin"

	"route-manager/internal/models"
	"route-manager/internal/netutil"
	"route-manager/internal/routecmd"
)

func (s *Server) hResolveConflicts(c *gin.Context) {
	if !s.elevated {
		fail(c, 403, "当前不是管理员权限，无法解决路由冲突")
		return
	}
	var body struct {
		SegmentIDs []uint `json:"segment_ids"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || len(body.SegmentIDs) == 0 {
		fail(c, 400, "请选择需要解决冲突的网段")
		return
	}
	current := s.eng.Reconcile()
	conflicts := make(map[uint]bool)
	for _, entry := range current.Entries {
		if entry.Status == "CONFLICT" {
			conflicts[entry.SegmentID] = true
		}
	}
	seen := make(map[uint]bool, len(body.SegmentIDs))
	results := make([]gin.H, 0, len(body.SegmentIDs))
	for _, id := range body.SegmentIDs {
		if id == 0 || seen[id] {
			continue
		}
		seen[id] = true
		if !conflicts[id] {
			results = append(results, gin.H{"segment_id": id, "ok": false, "error": "该网段当前没有路由冲突"})
			continue
		}
		if err := s.eng.ForceReplace(id); err != nil {
			results = append(results, gin.H{"segment_id": id, "ok": false, "error": err.Error()})
		} else {
			results = append(results, gin.H{"segment_id": id, "ok": true})
		}
	}
	status := s.eng.Reconcile()
	ok(c, gin.H{"results": results, "status": status})
}

func (s *Server) hRouteStatus(c *gin.Context) {
	ok(c, s.eng.Status())
}

func (s *Server) hRouteSync(c *gin.Context) {
	if !s.elevated {
		fail(c, 403, "当前不是管理员权限，无法写入系统路由")
		return
	}
	ok(c, s.eng.Reconcile())
}

func (s *Server) hRouteActual(c *gin.Context) {
	active, persistent, err := routecmd.ReadRoutes()
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	ok(c, gin.H{"active": routeRows(active), "persistent": routeRows(persistent)})
}

func (s *Server) hDeleteActualRoute(c *gin.Context) {
	if !s.elevated {
		fail(c, 403, "当前不是管理员权限，无法删除系统路由")
		return
	}
	var body struct {
		Cidr    string `json:"cidr"`
		Gateway string `json:"gateway"`
		Store   string `json:"store"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.Cidr == "" || body.Gateway == "" || (body.Store != "active" && body.Store != "persistent") {
		fail(c, 400, "路由参数错误")
		return
	}
	active, persistent, err := routecmd.ReadRoutes()
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	rows := active
	if body.Store == "persistent" {
		rows = persistent
	}
	found := false
	for _, row := range rows {
		if row.Cidr() == body.Cidr && row.Gateway == body.Gateway {
			found = true
			break
		}
	}
	if !found {
		fail(c, 404, "路由不存在或已被删除")
		return
	}
	if err := routecmd.DeleteFromStore(body.Cidr, body.Gateway, body.Store); err != nil {
		fail(c, 500, "删除路由失败: "+err.Error())
		return
	}
	noContent(c)
}

// hClearPersistent 一键清空本应用创建的持久路由。
// 只清理 applied_routes 追踪的、或命中当前网段 CIDR 的持久路由；
// 跳过 on-link/0.0.0.0 等系统接口路由。清完后重放期望态，
// 仍在使用的网段会以“活动路由”方式重建，路由不会因清空而丢失。
func (s *Server) hClearPersistent(c *gin.Context) {
	if !s.elevated {
		fail(c, 403, "当前不是管理员权限，无法清空持久路由")
		return
	}
	_, persistent, err := routecmd.ReadRoutes()
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	// 目标集合：applied_routes 记录 + 当前配置的网段 CIDR。
	clearKeys := map[string]bool{}
	var applied []models.AppliedRoute
	s.db.Find(&applied)
	for _, r := range applied {
		clearKeys[r.Cidr+"\x00"+r.GatewayIP] = true
	}
	segCidrs := map[string]bool{}
	var segs []models.Segment
	s.db.Find(&segs)
	for _, seg := range segs {
		segCidrs[seg.Cidr] = true
	}
	cleared := 0
	failures := []gin.H{}
	for _, r := range persistent {
		if r.Gateway == "on-link" || r.Gateway == "0.0.0.0" {
			continue // 系统接口路由，绝不动
		}
		cidr := r.Cidr()
		if !clearKeys[cidr+"\x00"+r.Gateway] && !segCidrs[cidr] {
			continue
		}
		dest, mask, err := netutil.SplitCIDR(cidr)
		if err != nil {
			continue
		}
		if err := routecmd.Delete(r.Gateway, dest, mask); err != nil {
			failures = append(failures, gin.H{"cidr": cidr, "gateway": r.Gateway, "error": err.Error()})
			continue
		}
		cleared++
		s.db.Where("cidr = ? AND gateway_ip = ?", cidr, r.Gateway).Delete(&models.AppliedRoute{})
	}
	// 清完后重放期望态：仍在使用的网段重新以活动路由方式添加。
	status := s.eng.Reconcile()
	ok(c, gin.H{"cleared": cleared, "failures": failures, "status": status})
}

func routeRows(rows []routecmd.RouteRow) []gin.H {
	out := make([]gin.H, 0, len(rows))
	for _, r := range rows {
		out = append(out, gin.H{"cidr": r.Cidr(), "gateway": r.Gateway, "interface": r.Interface, "metric": r.Metric})
	}
	return out
}
