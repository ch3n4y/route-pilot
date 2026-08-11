package sync

import (
	"sync"
	"time"

	"gorm.io/gorm"

	"route-manager/internal/models"
	"route-manager/internal/netutil"
	"route-manager/internal/routecmd"
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
	Syncing    bool            `json:"syncing"`
	LastSyncAt time.Time       `json:"last_sync_at"`
	Desired    []DesiredEntry  `json:"desired"`
	Entries    []Entry         `json:"entries"`
	Summary    map[string]int  `json:"summary"`
}

type Engine struct {
	db      *gorm.DB
	mu      sync.Mutex
	result  Result
	syncing bool
}

func New(gdb *gorm.DB) *Engine {
	return &Engine{db: gdb}
}

func (e *Engine) Status() Result {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.result
}

// RequestSync 异步触发一次 reconcile（已有在进行则跳过）。
func (e *Engine) RequestSync() {
	e.mu.Lock()
	if e.syncing {
		e.mu.Unlock()
		return
	}
	e.syncing = true
	e.mu.Unlock()
	go func() {
		res := e.Reconcile()
		e.mu.Lock()
		e.result = res
		e.syncing = false
		e.mu.Unlock()
	}()
}

// desired 期望态：启用网段 × 启用活动绑定 × 启用网关。
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

// actualMap 读系统路由表，返回 {cidr: gateway_ip}。
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
		m[r.Cidr()] = r.Gateway
	}
	return m, nil
}

// classify 纯逻辑：期望态 vs 实际态。
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

// Reconcile 全量同步：分类并应用缺失路由，清理本应用不再需要的路由。
func (e *Engine) Reconcile() Result {
	res := Result{Syncing: true, LastSyncAt: time.Now(), Summary: map[string]int{"ok": 0, "missing": 0, "conflict": 0, "error": 0}}
	des, err := desired(e.db)
	if err != nil {
		res.Summary["error"]++
		return res
	}
	actual, aerr := actualMap()
	if aerr != nil {
		actual = map[string]string{}
	}
	res.Desired = des
	res.Entries = classify(des, actual)
	for _, en := range res.Entries {
		res.Summary[en.Status]++
	}
	for i := range res.Entries {
		if res.Entries[i].Status != "MISSING" {
			continue
		}
		d := des[i]
		dest, mask, err := netutil.SplitCIDR(d.Cidr)
		if err != nil {
			res.Entries[i].Status = "ERROR"
			res.Entries[i].Error = err.Error()
			continue
		}
		ifIdx, ok := netutil.GatewayReachableOnInterface(d.GatewayIP)
		if !ok {
			ifIdx = 0 // 让 Windows 自动选接口；若真不可达，route 命令会报错
		}
		if err := routecmd.Add(d.GatewayIP, dest, mask, d.Metric, ifIdx); err != nil {
			res.Entries[i].Status = "ERROR"
			res.Entries[i].Error = err.Error()
			res.Summary["missing"]--
			res.Summary["error"]++
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

// ForceReplace 处理 CONFLICT：删除系统中现存的同网段路由，再按期望添加。
func (e *Engine) ForceReplace(segmentID uint) error {
	var b models.Binding
	if err := e.db.Where("segment_id = ? AND is_active = 1", segmentID).First(&b).Error; err != nil {
		return err
	}
	var seg models.Segment
	var gw models.Gateway
	if err := e.db.First(&seg, b.SegmentID).Error; err != nil {
		return err
	}
	if err := e.db.First(&gw, b.GatewayID).Error; err != nil {
		return err
	}
	// 找到系统中冲突的实际网关
	actual, err := actualMap()
	if err != nil {
		return err
	}
	if old, ok := actual[seg.Cidr]; ok && old != gw.GatewayIP {
		dest, mask, _ := netutil.SplitCIDR(seg.Cidr)
		if err := routecmd.Delete(old, dest, mask); err != nil {
			return err
		}
	}
	dest, mask, err := netutil.SplitCIDR(seg.Cidr)
	if err != nil {
		return err
	}
	ifIdx, _ := netutil.GatewayReachableOnInterface(gw.GatewayIP)
	if err := routecmd.Add(gw.GatewayIP, dest, mask, gw.Metric, ifIdx); err != nil {
		return err
	}
	e.upsertApplied(DesiredEntry{SegmentID: seg.ID, Cidr: seg.Cidr, GatewayIP: gw.GatewayIP, Metric: gw.Metric}, ifIdx)
	return nil
}

// upsertApplied 记录本应用创建的路由（用于安全清理）。
func (e *Engine) upsertApplied(d DesiredEntry, ifIdx int) {
	var r models.AppliedRoute
	if err := e.db.Where("cidr = ? AND gateway_ip = ?", d.Cidr, d.GatewayIP).First(&r).Error; err != nil {
		e.db.Create(&models.AppliedRoute{
			SegmentID: d.SegmentID, Cidr: d.Cidr, GatewayIP: d.GatewayIP,
			Metric: d.Metric, IfIndex: ifIdx, Status: "OK", LastSyncAt: time.Now(),
		})
		return
	}
	e.db.Model(&r).Updates(map[string]any{"status": "OK", "last_sync_at": time.Now(), "last_error": ""})
}

// cleanup 删除本应用创建但已不在期望态的路由。
func (e *Engine) cleanup(des []DesiredEntry) {
	want := map[string]bool{}
	for _, d := range des {
		want[d.Cidr] = true
	}
	var applied []models.AppliedRoute
	e.db.Find(&applied)
	for _, r := range applied {
		if want[r.Cidr] {
			continue
		}
		dest, mask, err := netutil.SplitCIDR(r.Cidr)
		if err != nil {
			continue
		}
		if err := routecmd.Delete(r.GatewayIP, dest, mask); err == nil {
			e.db.Delete(&r)
		}
	}
}
