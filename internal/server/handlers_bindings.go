package server

import (
	"github.com/gin-gonic/gin"

	"route-manager/internal/models"
)

func (s *Server) hBindings(c *gin.Context) {
	q := s.db.Model(&models.Binding{})
	if sid := c.Query("segment_id"); sid != "" {
		q = q.Where("segment_id = ?", atou(sid))
	}
	if gid := c.Query("gateway_id"); gid != "" {
		q = q.Where("gateway_id = ?", atou(gid))
	}
	var items []models.Binding
	q.Order("position").Find(&items)
	ok(c, gin.H{"items": items})
}

func (s *Server) hCreateBinding(c *gin.Context) {
	var body struct {
		SegmentID uint  `json:"segment_id"`
		GatewayID uint  `json:"gateway_id"`
		Position  int   `json:"position"`
		Enabled   *bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.SegmentID == 0 || body.GatewayID == 0 {
		fail(c, 400, "segment_id 和 gateway_id 必填")
		return
	}
	// 校验实体存在（glebarez/sqlite 无 FK 约束，应用层保证）
	var segCnt, gwCnt int64
	s.db.Model(&models.Segment{}).Where("id = ?", body.SegmentID).Count(&segCnt)
	s.db.Model(&models.Gateway{}).Where("id = ?", body.GatewayID).Count(&gwCnt)
	if segCnt == 0 || gwCnt == 0 {
		fail(c, 409, "网段或网关不存在")
		return
	}
	enabled := true
	if body.Enabled != nil {
		enabled = *body.Enabled
	}
	b := models.Binding{SegmentID: body.SegmentID, GatewayID: body.GatewayID, Position: body.Position, Enabled: enabled}
	if err := s.db.Create(&b).Error; err != nil {
		fail(c, 409, "绑定失败（可能已存在）: "+err.Error())
		return
	}
	s.requestSync()
	created(c, gin.H{"item": b})
}

func (s *Server) hUpdateBinding(c *gin.Context) {
	id := atou(c.Param("id"))
	if id == 0 {
		fail(c, 400, "无效 id")
		return
	}
	var body struct {
		Position *int  `json:"position"`
		Enabled  *bool `json:"enabled"`
		IsActive *bool `json:"is_active"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		fail(c, 400, "参数错误")
		return
	}
	var b models.Binding
	if err := s.db.First(&b, id).Error; err != nil {
		fail(c, 404, "绑定不存在")
		return
	}
	if body.IsActive != nil && *body.IsActive && !s.elevated {
		fail(c, 403, "当前不是管理员权限，无法切换系统路由")
		return
	}
	updates := map[string]any{}
	if body.Position != nil {
		updates["position"] = *body.Position
	}
	if body.Enabled != nil {
		updates["enabled"] = *body.Enabled
	}
	if body.IsActive != nil {
		if !*body.IsActive {
			updates["is_active"] = false
		}
	}
	if len(updates) > 0 {
		if err := s.db.Model(&b).Updates(updates).Error; err != nil {
			fail(c, 422, err.Error())
			return
		}
	}
	if body.IsActive != nil && *body.IsActive {
		if err := s.activateBinding(b.SegmentID, b.GatewayID); err != nil {
			fail(c, 409, "切换失败: "+err.Error())
			return
		}
		s.reconcileAfterSwitch()
	} else {
		s.requestSync()
	}
	_ = s.db.First(&b, id).Error
	ok(c, gin.H{"item": b})
}

func (s *Server) hDeleteBinding(c *gin.Context) {
	id := atou(c.Param("id"))
	if id == 0 {
		fail(c, 400, "无效 id")
		return
	}
	if err := s.db.Delete(&models.Binding{}, id).Error; err != nil {
		fail(c, 500, err.Error())
		return
	}
	s.requestSync()
	noContent(c)
}

func (s *Server) hSetActive(c *gin.Context) {
	if !s.elevated {
		fail(c, 403, "当前不是管理员权限，无法切换系统路由")
		return
	}
	var body struct {
		SegmentID uint `json:"segment_id"`
		GatewayID uint `json:"gateway_id"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		fail(c, 400, "参数错误")
		return
	}
	if err := s.activateBinding(body.SegmentID, body.GatewayID); err != nil {
		fail(c, 409, "切换失败: "+err.Error())
		return
	}
	res := s.reconcileAfterSwitch()
	ok(c, gin.H{"ok": true, "status": res})
}
