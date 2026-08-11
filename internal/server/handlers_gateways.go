package server

import (
	"net"

	"github.com/gin-gonic/gin"

	"route-manager/internal/models"
	"route-manager/internal/netutil"
)

type gatewayWithMeta struct {
	models.Gateway
	UsedBy []uint `json:"used_by"`
}

func (s *Server) hGateways(c *gin.Context) {
	var gws []models.Gateway
	s.db.Order("id").Find(&gws)
	var binds []models.Binding
	s.db.Find(&binds)
	usedBy := map[uint][]uint{}
	for _, b := range binds {
		if !b.Enabled {
			continue
		}
		usedBy[b.GatewayID] = append(usedBy[b.GatewayID], b.SegmentID)
	}
	items := make([]gatewayWithMeta, 0, len(gws))
	for _, g := range gws {
		items = append(items, gatewayWithMeta{Gateway: g, UsedBy: usedBy[g.ID]})
	}
	ok(c, gin.H{"items": items})
}

type gatewayBody struct {
	Name        string `json:"name"`
	GatewayIP   string `json:"gateway_ip"`
	Interface   string `json:"interface"`
	IfIndex     int    `json:"ifindex"`
	Metric      int    `json:"metric"`
	Description string `json:"description"`
	Enabled     *bool  `json:"enabled"`
}

func (s *Server) hCreateGateway(c *gin.Context) {
	var body gatewayBody
	if err := c.ShouldBindJSON(&body); err != nil || body.Name == "" || body.GatewayIP == "" {
		fail(c, 400, "名称和网关 IP 必填")
		return
	}
	if net.ParseIP(body.GatewayIP) == nil || net.ParseIP(body.GatewayIP).To4() == nil {
		fail(c, 422, "网关必须是合法 IPv4")
		return
	}
	enabled := true
	if body.Enabled != nil {
		enabled = *body.Enabled
	}
	gw := models.Gateway{Name: body.Name, GatewayIP: body.GatewayIP, Interface: body.Interface,
		IfIndex: body.IfIndex, Metric: body.Metric, Description: body.Description, Enabled: enabled}
	if err := s.db.Create(&gw).Error; err != nil {
		fail(c, 422, err.Error())
		return
	}
	created(c, gin.H{"item": gw})
}

func (s *Server) hUpdateGateway(c *gin.Context) {
	id := atou(c.Param("id"))
	if id == 0 {
		fail(c, 400, "无效 id")
		return
	}
	var body gatewayBody
	if err := c.ShouldBindJSON(&body); err != nil || body.Name == "" || body.GatewayIP == "" {
		fail(c, 400, "名称和网关 IP 必填")
		return
	}
	if net.ParseIP(body.GatewayIP) == nil || net.ParseIP(body.GatewayIP).To4() == nil {
		fail(c, 422, "网关必须是合法 IPv4")
		return
	}
	var gw models.Gateway
	if err := s.db.First(&gw, id).Error; err != nil {
		fail(c, 404, "网关不存在")
		return
	}
	enabled := gw.Enabled
	if body.Enabled != nil {
		enabled = *body.Enabled
	}
	updates := map[string]any{"name": body.Name, "gateway_ip": body.GatewayIP, "interface": body.Interface,
		"ifindex": body.IfIndex, "metric": body.Metric, "description": body.Description, "enabled": enabled}
	if err := s.db.Model(&gw).Updates(updates).Error; err != nil {
		fail(c, 422, err.Error())
		return
	}
	s.eng.RequestSync()
	ok(c, gin.H{"item": gw})
}

func (s *Server) hDeleteGateway(c *gin.Context) {
	id := atou(c.Param("id"))
	if id == 0 {
		fail(c, 400, "无效 id")
		return
	}
	// glebarez/sqlite 不生成 FK 约束，级联由应用层保证
	if err := s.db.Where("gateway_id = ?", id).Delete(&models.Binding{}).Error; err != nil {
		fail(c, 500, err.Error())
		return
	}
	if err := s.db.Delete(&models.Gateway{}, id).Error; err != nil {
		fail(c, 500, err.Error())
		return
	}
	s.eng.RequestSync()
	noContent(c)
}

func (s *Server) hNetworkInterfaces(c *gin.Context) {
	ifaces, err := netutil.LocalInterfaces()
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	ok(c, gin.H{"interfaces": ifaces})
}
