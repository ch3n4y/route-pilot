package sync

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"gorm.io/gorm"

	"route-manager/internal/models"
	"route-manager/internal/netutil"
	"route-manager/internal/routecmd"
)

type DesiredEntry struct {
	BindingID   uint   `json:"binding_id"`
	SegmentID   uint   `json:"segment_id"`
	SegmentName string `json:"segment_name"`
	Cidr        string `json:"cidr"`
	GatewayIP   string `json:"gateway_ip"`
	Metric      int    `json:"metric"`
	IfIndex     int    `json:"ifindex"`
	Position    int    `json:"position"`
}

type Entry struct {
	BindingID   uint   `json:"binding_id"`
	SegmentID   uint   `json:"segment_id"`
	SegmentName string `json:"segment_name"`
	Cidr        string `json:"cidr"`
	GatewayIP   string `json:"gateway_ip"`
	Metric      int    `json:"metric"` // 有效跃点（基础跃点 + 覆盖提升）
	Status      string `json:"status"` // OK|MISSING|CONFLICT|MISMATCH|ERROR
	Error       string `json:"error"`
}

type Result struct {
	Syncing    bool           `json:"syncing"`
	LastSyncAt time.Time      `json:"last_sync_at"`
	Desired    []DesiredEntry `json:"desired"`
	Entries    []Entry        `json:"entries"`
	Summary    map[string]int `json:"summary"`
}

type Engine struct {
	db      *gorm.DB
	mu      sync.Mutex
	result  Result
	syncing bool
	pending bool
}

func New(gdb *gorm.DB) *Engine {
	return &Engine{db: gdb}
}

func (e *Engine) Status() Result {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.result
}

// RequestSync 异步触发一次 reconcile。同步期间的新变更会合并为下一轮，
// 这样网关批量选择/排序无需等待系统路由命令完成。
func (e *Engine) RequestSync() {
	e.mu.Lock()
	if e.syncing {
		e.pending = true
		e.mu.Unlock()
		return
	}
	e.syncing = true
	e.mu.Unlock()
	go e.Reconcile()
}

// desired 期望态：启用网段 × 启用活动绑定 × 启用网关。跃点取网段基础跃点，并按包含关系提升。
func desired(db *gorm.DB) ([]DesiredEntry, error) {
	var rows []DesiredEntry
	err := db.Table("segments").
		Select("bindings.id AS binding_id, bindings.position, segments.id AS segment_id, segments.cidr AS segment_name, segments.cidr, segments.metric, gateways.gateway_ip, gateways.ifindex").
		Joins("JOIN bindings ON bindings.segment_id = segments.id AND bindings.enabled = 1").
		Joins("JOIN gateways ON gateways.id = bindings.gateway_id AND gateways.enabled = 1").
		Where("segments.enabled = 1").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	applyEffectiveMetrics(rows)
	return rows, nil
}

// routeActual 系统路由表中的一条路由（网关 + 跃点），供跃点漂移对比。
type routeActual struct {
	Cidr        string
	Gateway     string
	Metric      int
	MetricKnown bool
}

// actualMap 读系统路由表，返回 {cidr: {网关, 跃点}}。
func actualMap() (map[string]routeActual, error) {
	// 不使用 route print 的 Metric：它可能叠加接口跃点，不能和 route add
	// 的 METRIC 参数直接比较。Get-NetRoute 的 RouteMetric 才是路由自身跃点。
	active, err := routecmd.ReadActiveRoutes()
	metricKnown := err == nil
	if err != nil {
		// 旧版系统可能没有 NetTCPIP / Get-NetRoute。此时 route print 仍可用于
		// 判断网关是否存在，但它展示的跃点可能叠加接口跃点，不能触发重写。
		active, _, err = routecmd.ReadRoutes()
		if err != nil {
			return nil, err
		}
	}
	m := map[string]routeActual{}
	for _, r := range active {
		if r.Gateway == "on-link" {
			continue
		}
		m[routeKey(r.Cidr(), r.Gateway)] = routeActual{Cidr: r.Cidr(), Gateway: r.Gateway, Metric: r.Metric, MetricKnown: metricKnown}
	}
	return m, nil
}

// applyEffectiveMetrics 先按网段包含关系提升基础跃点，再按同网段 binding 的
// position 递增跃点；position 越小，网关优先级越高。
// 19.25.0.0/16 盖住 19.25.22.0/24 -> /16 +1 变 2；再加 19.25.22.100/32 -> /24 +1、/16 再 +1 变 3。
// 被覆盖的更具体网段优先级更高，覆盖方跃点自动抬高，符合"跃点小的先走"的直觉。
func applyEffectiveMetrics(rows []DesiredEntry) {
	base := make(map[string]int)
	for _, row := range rows {
		if current, ok := base[row.Cidr]; !ok || row.Metric < current {
			base[row.Cidr] = row.Metric
		}
	}
	for outer := range base {
		for inner := range base {
			if outer != inner && netutil.Contains(outer, inner) {
				base[outer]++
			}
		}
	}
	for i := range rows {
		rows[i].Metric = base[rows[i].Cidr]
	}
	groups := make(map[string][]int)
	for i := range rows {
		groups[rows[i].Cidr] = append(groups[rows[i].Cidr], i)
	}
	for cidr, indexes := range groups {
		sort.SliceStable(indexes, func(i, j int) bool {
			a, b := rows[indexes[i]], rows[indexes[j]]
			if a.Position != b.Position {
				return a.Position < b.Position
			}
			return a.BindingID < b.BindingID
		})
		for rank, index := range indexes {
			rows[index].Metric = base[cidr] + rank
		}
	}
}

// activeMetrics 返回 {segment_id: 有效跃点}，与 desired() 同源（含覆盖提升）。
func activeMetrics(gdb *gorm.DB) (map[uint]int, error) {
	rows, err := desired(gdb)
	if err != nil {
		return nil, err
	}
	m := make(map[uint]int, len(rows))
	for _, r := range rows {
		m[r.SegmentID] = r.Metric
	}
	return m, nil
}

// classify 纯逻辑：期望态 vs 实际态。网关一致但跃点不符 -> MISMATCH（引擎自动重写）。
func classify(des []DesiredEntry, actual map[string]routeActual) []Entry {
	var out []Entry
	for _, d := range des {
		e := Entry{BindingID: d.BindingID, SegmentID: d.SegmentID, SegmentName: d.SegmentName, Cidr: d.Cidr, GatewayIP: d.GatewayIP, Metric: d.Metric}
		ar, ok := actual[routeKey(d.Cidr, d.GatewayIP)]
		if !ok {
			e.Status = "MISSING"
		} else if ar.MetricKnown && ar.Metric != d.Metric {
			e.Status = "MISMATCH"
			e.Error = fmt.Sprintf("系统路由跃点为 %d，应为 %d，将自动重写", ar.Metric, d.Metric)
		} else {
			e.Status = "OK"
		}
		out = append(out, e)
	}
	return out
}

// Reconcile 全量同步：结果写回缓存，Status()/前端轮询可拿到最新状态。
func (e *Engine) Reconcile() Result {
	e.mu.Lock()
	res := e.reconcile()
	e.result = res
	runAgain := e.pending
	e.pending = false
	e.syncing = false
	e.mu.Unlock()
	if runAgain {
		e.RequestSync()
	}
	return res
}

func (e *Engine) reconcile() Result {
	res := Result{Syncing: true, LastSyncAt: time.Now(), Summary: map[string]int{"ok": 0, "missing": 0, "conflict": 0, "mismatch": 0, "error": 0}}
	des, err := desired(e.db)
	if err != nil {
		res.Summary["error"]++
		return res
	}
	actual, aerr := actualMap()
	if aerr != nil {
		// 无法确认系统实际态时绝不能盲目 add，否则可能覆盖或叠加用户路由。
		res.Desired = des
		for _, d := range des {
			res.Entries = append(res.Entries, Entry{
				SegmentID: d.SegmentID, SegmentName: d.SegmentName, Cidr: d.Cidr,
				GatewayIP: d.GatewayIP, Status: "ERROR", Error: "读取系统路由失败: " + aerr.Error(),
			})
			res.Summary["error"]++
		}
		res.Syncing = false
		return res
	}
	// 确认实际态可读后，再移除本应用创建但已不再匹配期望态的旧路由。
	// 网关 IP 或 CIDR 修改后若不先清理，旧路由会被误判成用户手工冲突。
	if e.cleanup(des) {
		actual, aerr = actualMap()
		if aerr != nil {
			res.Summary["error"]++
			res.Syncing = false
			return res
		}
	}
	// 同一网段只能由本程序声明的网关集合接管。发现同 CIDR 的未追踪
	// 系统路由时先删除，再由下面的期望态完整重建，避免手工路由与本程序
	// 的多网关优先级混用。
	if changed, err := e.removeUnmanagedSameCIDR(des, actual); err != nil {
		res.Desired = des
		for _, d := range des {
			res.Entries = append(res.Entries, Entry{BindingID: d.BindingID, SegmentID: d.SegmentID, SegmentName: d.SegmentName, Cidr: d.Cidr, GatewayIP: d.GatewayIP, Status: "ERROR", Error: err.Error()})
			res.Summary["error"]++
		}
		res.Syncing = false
		return res
	} else if changed {
		actual, aerr = actualMap()
		if aerr != nil {
			res.Summary["error"]++
			res.Syncing = false
			return res
		}
	}
	if _, err := e.removeUnmanagedPersistentSameCIDR(des); err != nil {
		res.Desired = des
		for _, d := range des {
			res.Entries = append(res.Entries, Entry{BindingID: d.BindingID, SegmentID: d.SegmentID, SegmentName: d.SegmentName, Cidr: d.Cidr, GatewayIP: d.GatewayIP, Status: "ERROR", Error: err.Error()})
			res.Summary["error"]++
		}
		res.Syncing = false
		return res
	}
	res.Desired = des
	res.Entries = classify(des, actual)
	// 统一 summary key（小写）：OK->ok, MISSING->missing, CONFLICT->conflict, MISMATCH->mismatch, ERROR->error
	lower := map[string]string{"OK": "ok", "MISSING": "missing", "CONFLICT": "conflict", "MISMATCH": "mismatch", "ERROR": "error"}
	for _, en := range res.Entries {
		res.Summary[lower[en.Status]]++
	}
	for i := range res.Entries {
		if res.Entries[i].Status != "MISSING" && res.Entries[i].Status != "MISMATCH" {
			continue
		}
		d := des[i]
		from := "missing"
		if res.Entries[i].Status == "MISMATCH" {
			from = "mismatch"
		}
		dest, mask, err := netutil.SplitCIDR(d.Cidr)
		if err != nil {
			res.Entries[i].Status = "ERROR"
			res.Entries[i].Error = err.Error()
			res.Summary[from]--
			res.Summary["error"]++
			continue
		}
		// 跃点不符：先删系统里旧跃点的路由，再按正确跃点重建。
		if from == "mismatch" {
			if ar, ok := actual[routeKey(d.Cidr, d.GatewayIP)]; ok {
				_ = routecmd.Delete(ar.Gateway, dest, mask)
			}
		}
		ifIdx := d.IfIndex
		if ifIdx <= 0 {
			ifIdx, _ = netutil.GatewayReachableOnInterface(d.GatewayIP)
		}
		if err := routecmd.Add(d.GatewayIP, dest, mask, d.Metric, ifIdx); err != nil {
			res.Entries[i].Status = "ERROR"
			res.Entries[i].Error = err.Error()
			res.Summary[from]--
			res.Summary["error"]++
			continue
		}
		res.Entries[i].Status = "OK"
		res.Entries[i].Error = ""
		res.Summary[from]--
		res.Summary["ok"]++
		e.upsertApplied(d, ifIdx)
	}
	res.Syncing = false
	return res
}

// ForceReplace 处理 CONFLICT：删除系统中现存的同网段路由，再按期望添加。
func (e *Engine) ForceReplace(segmentID uint) error {
	var b models.Binding
	if err := e.db.Where("segment_id = ? AND enabled = 1", segmentID).Order("position, id").First(&b).Error; err != nil {
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
	dest, mask, err := netutil.SplitCIDR(seg.Cidr)
	if err != nil {
		return err
	}
	ifIdx := gw.IfIndex
	if ifIdx <= 0 {
		ifIdx, _ = netutil.GatewayReachableOnInterface(gw.GatewayIP)
	}
	// 有效跃点：网段基础跃点 + 覆盖提升（与 desired() 同源，保证一致）。
	metric := max(seg.Metric, 1)
	if met, err := activeMetrics(e.db); err == nil {
		if m, ok := met[seg.ID]; ok {
			metric = m
		}
	}
	if err := routecmd.Add(gw.GatewayIP, dest, mask, metric, ifIdx); err != nil {
		return err
	}
	e.upsertApplied(DesiredEntry{SegmentID: seg.ID, Cidr: seg.Cidr, GatewayIP: gw.GatewayIP, Metric: metric}, ifIdx)
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
func (e *Engine) cleanup(des []DesiredEntry) bool {
	want := map[string]bool{}
	changed := false
	for _, d := range des {
		want[routeKey(d.Cidr, d.GatewayIP)] = true
	}
	var applied []models.AppliedRoute
	e.db.Find(&applied)
	for _, r := range applied {
		if want[routeKey(r.Cidr, r.GatewayIP)] {
			continue
		}
		dest, mask, err := netutil.SplitCIDR(r.Cidr)
		if err != nil {
			continue
		}
		if err := routecmd.Delete(r.GatewayIP, dest, mask); err == nil {
			e.db.Delete(&r)
			changed = true
		}
	}
	return changed
}

// removeUnmanagedSameCIDR 删除与当前配置 CIDR 相同、但不在 applied_routes
// 追踪集合里的系统路由。即使下一跳碰巧与期望值相同，未追踪路由也会重建为
// 本程序管理的路由，确保后续多网关优先级可预测。
func (e *Engine) removeUnmanagedSameCIDR(des []DesiredEntry, actual map[string]routeActual) (bool, error) {
	wantedCIDR := make(map[string]bool, len(des))
	for _, d := range des {
		wantedCIDR[d.Cidr] = true
	}
	var applied []models.AppliedRoute
	if err := e.db.Find(&applied).Error; err != nil {
		return false, err
	}
	managed := make(map[string]bool, len(applied))
	for _, r := range applied {
		managed[routeKey(r.Cidr, r.GatewayIP)] = true
	}
	changed := false
	for key, route := range actual {
		if !wantedCIDR[route.Cidr] || managed[key] {
			continue
		}
		dest, mask, err := netutil.SplitCIDR(route.Cidr)
		if err != nil {
			return changed, err
		}
		if err := routecmd.Delete(route.Gateway, dest, mask); err != nil {
			return changed, fmt.Errorf("删除非本程序管理的同网段路由 %s via %s 失败: %w", route.Cidr, route.Gateway, err)
		}
		changed = true
	}
	return changed, nil
}

// removeUnmanagedPersistentSameCIDR 同样清除旧版/手工遗留的永久路由，防止其在
// 重启后重新出现并破坏当前网段的多网关优先级。
func (e *Engine) removeUnmanagedPersistentSameCIDR(des []DesiredEntry) (bool, error) {
	wantedCIDR := make(map[string]bool, len(des))
	for _, d := range des {
		wantedCIDR[d.Cidr] = true
	}
	var applied []models.AppliedRoute
	if err := e.db.Find(&applied).Error; err != nil {
		return false, err
	}
	managed := make(map[string]bool, len(applied))
	for _, r := range applied {
		managed[routeKey(r.Cidr, r.GatewayIP)] = true
	}
	_, persistent, err := routecmd.ReadRoutes()
	if err != nil {
		return false, err
	}
	changed := false
	for _, route := range persistent {
		cidr := route.Cidr()
		key := routeKey(cidr, route.Gateway)
		if !wantedCIDR[cidr] || managed[key] || route.Gateway == "on-link" {
			continue
		}
		if err := routecmd.DeleteFromStore(cidr, route.Gateway, "persistent"); err != nil {
			return changed, fmt.Errorf("删除非本程序管理的永久路由 %s via %s 失败: %w", cidr, route.Gateway, err)
		}
		changed = true
	}
	return changed, nil
}

func routeKey(cidr, gateway string) string {
	return fmt.Sprintf("%s\x00%s", cidr, gateway)
}
