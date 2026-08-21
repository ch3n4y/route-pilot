package server

import (
	"fmt"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"route-manager/internal/models"
	"route-manager/internal/netutil"
)

// segmentWithMeta 前端需要的附加字段：活动网关 id + 候选绑定列表（含网关名）。
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
	Position    int    `json:"position"`
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
			vm.Bindings = append(vm.Bindings, bindingVM{ID: b.ID, GatewayID: b.GatewayID, GatewayName: gwName[b.GatewayID], IsActive: b.IsActive, Enabled: b.Enabled, Position: b.Position})
		}
		items = append(items, vm)
	}
	ok(c, gin.H{"items": items})
}

type segmentBody struct {
	Cidr        string `json:"cidr"`
	Metric      int    `json:"metric"` // 基础跃点，默认 1；被更具体网段覆盖时由引擎自动提升
	Description string `json:"description"`
	Enabled     *bool  `json:"enabled"`
}

func (s *Server) hCreateSegment(c *gin.Context) {
	var body segmentBody
	if err := c.ShouldBindJSON(&body); err != nil || body.Cidr == "" {
		fail(c, 400, "网段必填")
		return
	}
	network, mask, err := netutil.CanonicalCIDR(body.Cidr)
	if err != nil {
		fail(c, 422, err.Error())
		return
	}
	metric := max(body.Metric, 1)
	enabled := true
	if body.Enabled != nil {
		enabled = *body.Enabled
	}
	// 允许重叠/嵌套网段：19.25.22.0/24 可作为 19.25.0.0/16 的更具体路由存在。
	// 精确重复仍被 cidr UNIQUE 拒绝。
	seg := models.Segment{Name: network, Cidr: network, Netmask: mask, Metric: metric, Description: body.Description, Enabled: enabled}
	if err := s.db.Create(&seg).Error; err != nil {
		fail(c, 422, "创建失败: "+err.Error())
		return
	}
	s.requestSync()
	created(c, gin.H{"item": seg})
}

func (s *Server) hUpdateSegment(c *gin.Context) {
	id := atou(c.Param("id"))
	if id == 0 {
		fail(c, 400, "无效 id")
		return
	}
	var body segmentBody
	if err := c.ShouldBindJSON(&body); err != nil || body.Cidr == "" {
		fail(c, 400, "网段必填")
		return
	}
	network, mask, err := netutil.CanonicalCIDR(body.Cidr)
	if err != nil {
		fail(c, 422, err.Error())
		return
	}
	var seg models.Segment
	if err := s.db.First(&seg, id).Error; err != nil {
		fail(c, 404, "网段不存在")
		return
	}
	enabled := seg.Enabled
	if body.Enabled != nil {
		enabled = *body.Enabled
	}
	updates := map[string]any{"name": network, "cidr": network, "netmask": mask, "description": body.Description, "enabled": enabled}
	if body.Metric >= 1 {
		updates["metric"] = body.Metric
	}
	if err := s.db.Model(&seg).Updates(updates).Error; err != nil {
		fail(c, 422, err.Error())
		return
	}
	// GORM 的 map 更新不会自动刷新传入 struct，重新读取后再返回真实持久态。
	_ = s.db.First(&seg, id).Error
	s.requestSync()
	ok(c, gin.H{"item": seg})
}

func (s *Server) hDeleteSegment(c *gin.Context) {
	id := atou(c.Param("id"))
	if id == 0 {
		fail(c, 400, "无效 id")
		return
	}
	// glebarez/sqlite 不生成 FK 约束，级联由应用层保证
	if err := s.db.Where("segment_id = ?", id).Delete(&models.Binding{}).Error; err != nil {
		fail(c, 500, err.Error())
		return
	}
	if err := s.db.Delete(&models.Segment{}, id).Error; err != nil {
		fail(c, 500, err.Error())
		return
	}
	s.requestSync()
	noContent(c)
}

// activateBinding 事务内：清该段活动，再把目标网关设为活动（upsert 绑定）。
// 校验网段/网关存在；不存在返回错误。
func (s *Server) activateBinding(segmentID, gatewayID uint) error {
	if segmentID == 0 || gatewayID == 0 {
		return gorm.ErrInvalidTransaction
	}
	var segCnt, gwCnt int64
	if err := s.db.Model(&models.Segment{}).Where("id = ?", segmentID).Count(&segCnt).Error; err != nil {
		return err
	}
	if segCnt == 0 {
		return fmt.Errorf("网段 %d 不存在", segmentID)
	}
	if err := s.db.Model(&models.Gateway{}).Where("id = ? AND enabled = 1", gatewayID).Count(&gwCnt).Error; err != nil {
		return err
	}
	if gwCnt == 0 {
		return fmt.Errorf("网关 %d 不存在或已禁用", gatewayID)
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		var b models.Binding
		err := tx.Where("segment_id = ? AND gateway_id = ?", segmentID, gatewayID).First(&b).Error
		if err == gorm.ErrRecordNotFound {
			return tx.Create(&models.Binding{SegmentID: segmentID, GatewayID: gatewayID, Enabled: true}).Error
		}
		if err != nil {
			return err
		}
		return tx.Model(&b).Update("enabled", true).Error
	})
}

func (s *Server) hSwitchSegment(c *gin.Context) {
	if !s.elevated {
		fail(c, 403, "当前不是管理员权限，无法切换系统路由")
		return
	}
	id := atou(c.Param("id"))
	if id == 0 {
		fail(c, 400, "无效 id")
		return
	}
	var body struct {
		GatewayID uint `json:"gateway_id"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		fail(c, 400, "参数错误")
		return
	}
	if err := s.activateBinding(id, body.GatewayID); err != nil {
		fail(c, 409, "切换失败: "+err.Error())
		return
	}
	res := s.reconcileAfterSwitch()
	ok(c, gin.H{"ok": true, "status": res})
}

func (s *Server) hBatchSwitch(c *gin.Context) {
	if !s.elevated {
		fail(c, 403, "当前不是管理员权限，无法切换系统路由")
		return
	}
	var body struct {
		SegmentIDs []uint `json:"segment_ids"`
		GatewayID  uint   `json:"gateway_id"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || len(body.SegmentIDs) == 0 {
		fail(c, 400, "参数错误")
		return
	}
	results := []gin.H{}
	for _, sid := range body.SegmentIDs {
		if err := s.activateBinding(sid, body.GatewayID); err != nil {
			results = append(results, gin.H{"segment_id": sid, "ok": false, "error": err.Error()})
		} else {
			results = append(results, gin.H{"segment_id": sid, "ok": true})
		}
	}
	res := s.reconcileAfterSwitch()
	ok(c, gin.H{"results": results, "status": res})
}
