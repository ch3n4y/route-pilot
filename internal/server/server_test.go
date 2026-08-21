package server

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"route-manager/internal/config"
	"route-manager/internal/db"
	"route-manager/internal/models"
	"route-manager/internal/sync"
)

func do(t *testing.T, eng *gin.Engine, method, path, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	eng.ServeHTTP(w, req)
	return w
}

func TestResolveConflictsRequiresSelection(t *testing.T) {
	r, _ := newTestServer(t)
	w := do(t, r, "POST", "/api/routes/resolve-conflicts", "", map[string]any{"segment_ids": []uint{}})
	if w.Code != 400 {
		t.Fatalf("empty conflict selection should be rejected: %d %s", w.Code, w.Body.String())
	}
}

func TestClearPersistentRequiresElevation(t *testing.T) {
	gdb, err := db.Open(&config.AppConfig{DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sqlDB, _ := gdb.DB(); sqlDB.Close() })
	eng := sync.New(gdb)
	r := New(gdb, eng, &config.AppConfig{Host: "127.0.0.1", DataDir: t.TempDir()}, "test", false, "38254", nil)
	w := do(t, r, "POST", "/api/routes/clear-persistent", "", nil)
	if w.Code != 403 {
		t.Fatalf("clear-persistent without elevation should 403: %d %s", w.Code, w.Body.String())
	}
}

func TestCreateGatewayRejectsUnreachableInterface(t *testing.T) {
	r, _ := newTestServer(t)
	originalInterfaceCheck := gatewayInterfaceContainsIP
	gatewayInterfaceContainsIP = func(int, string) bool { return false }
	t.Cleanup(func() { gatewayInterfaceContainsIP = originalInterfaceCheck })
	w := do(t, r, "POST", "/api/gateways", "", map[string]any{
		"name": "GW-LAN", "gateway_ip": "192.168.1.2", "ifindex": 12,
	})
	if w.Code != 422 {
		t.Fatalf("unreachable interface should be rejected: %d %s", w.Code, w.Body.String())
	}
}

func newTestServer(t *testing.T) (*gin.Engine, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	originalInterfaceCheck := gatewayInterfaceContainsIP
	gatewayInterfaceContainsIP = func(int, string) bool { return true }
	t.Cleanup(func() { gatewayInterfaceContainsIP = originalInterfaceCheck })
	gdb, err := db.Open(&config.AppConfig{DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sqlDB, _ := gdb.DB(); sqlDB.Close() })
	eng := sync.New(gdb)
	r := New(gdb, eng, &config.AppConfig{Host: "127.0.0.1", DataDir: t.TempDir()}, "test", true, "38254", nil)
	return r, "tok"
}

func TestCRUDFlowWithoutAuthentication(t *testing.T) {
	r, _ := newTestServer(t)
	token := ""
	w := do(t, r, "GET", "/api/segments", token, nil)
	if w.Code != 200 {
		t.Fatalf("local API should not require authentication: %d", w.Code)
	}

	// 3. create segment + duplicate rejected
	w = do(t, r, "POST", "/api/segments", token, map[string]any{"name": "办公网", "cidr": "10.1.2.3/8"})
	if w.Code != 201 {
		t.Fatalf("create segment: %d %s", w.Code, w.Body.String())
	}
	var segResp struct {
		Item struct {
			ID      uint   `json:"id"`
			Cidr    string `json:"cidr"`
			Netmask string `json:"netmask"`
		} `json:"item"`
	}
	json.Unmarshal(w.Body.Bytes(), &segResp)
	if segResp.Item.Cidr != "10.0.0.0/8" || segResp.Item.Netmask != "255.0.0.0" {
		t.Fatalf("canonical cidr wrong: %+v", segResp.Item)
	}
	segID := segResp.Item.ID
	w = do(t, r, "POST", "/api/segments", token, map[string]any{"name": "重复", "cidr": "10.0.0.0/8"})
	if w.Code != 422 {
		t.Fatalf("duplicate segment should 422: %d %s", w.Code, w.Body.String())
	}
	w = do(t, r, "POST", "/api/segments", token, map[string]any{"name": "重叠", "cidr": "10.5.0.0/16", "metric": 3})
	if w.Code != 201 {
		t.Fatalf("nested/overlapping segment should now be allowed: %d %s", w.Code, w.Body.String())
	}
	if !bytes.Contains(w.Body.Bytes(), []byte(`"metric":3`)) {
		t.Fatalf("segment metric should be persisted: %s", w.Body.String())
	}
	w = do(t, r, "POST", "/api/segments", token, map[string]any{"cidr": "192.0.2.15", "description": "主机路由"})
	if w.Code != 201 || !bytes.Contains(w.Body.Bytes(), []byte(`"cidr":"192.0.2.15/32"`)) {
		t.Fatalf("plain IPv4 should become /32: %d %s", w.Code, w.Body.String())
	}

	// 4. create gateway + invalid ip rejected
	w = do(t, r, "POST", "/api/gateways", token, map[string]any{"name": "GW-LAN", "gateway_ip": "192.168.1.2", "ifindex": 12})
	if w.Code != 201 {
		t.Fatalf("create gateway: %d %s", w.Code, w.Body.String())
	}
	var gwResp struct {
		Item struct {
			ID uint `json:"id"`
		}
	}
	json.Unmarshal(w.Body.Bytes(), &gwResp)
	gwID := gwResp.Item.ID
	w = do(t, r, "GET", "/api/gateways", token, nil)
	if w.Code != 200 || !bytes.Contains(w.Body.Bytes(), []byte(`"used_by":[]`)) {
		t.Fatalf("unbound gateway used_by must be an empty array: %d %s", w.Code, w.Body.String())
	}
	w = do(t, r, "POST", "/api/gateways", token, map[string]any{"name": "bad", "gateway_ip": "999.1.1.1"})
	if w.Code != 422 {
		t.Fatalf("invalid gateway ip should 422: %d", w.Code)
	}

	// 5. bind candidate (not active -> no system route mutation)
	w = do(t, r, "POST", "/api/bindings", token, map[string]any{"segment_id": segID, "gateway_id": gwID})
	if w.Code != 201 {
		t.Fatalf("create binding: %d %s", w.Code, w.Body.String())
	}
	w = do(t, r, "GET", "/api/routes/status", token, nil)
	if w.Code != 200 {
		t.Fatalf("routes/status: %d", w.Code)
	}

	// 6. delete segment cascades
	w = do(t, r, "DELETE", "/api/segments/"+itoa(segID), token, nil)
	if w.Code != 204 {
		t.Fatalf("delete segment: %d %s", w.Code, w.Body.String())
	}
}

func itoa(n uint) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func TestDeleteCascadesBindings(t *testing.T) {
	r, _ := newTestServer(t)
	tk := ""

	// 建网段+网关+绑定
	w := do(t, r, "POST", "/api/segments", tk, map[string]any{"name": "s", "cidr": "10.0.0.0/8"})
	json.Unmarshal(w.Body.Bytes(), &struct{ Item struct{ ID uint } }{})
	var segResp struct{ Item struct{ ID uint } }
	json.Unmarshal(w.Body.Bytes(), &segResp)
	segID := segResp.Item.ID
	w = do(t, r, "POST", "/api/gateways", tk, map[string]any{"name": "g", "gateway_ip": "192.168.1.2", "ifindex": 12})
	var gwResp struct{ Item struct{ ID uint } }
	json.Unmarshal(w.Body.Bytes(), &gwResp)
	gwID := gwResp.Item.ID
	if w := do(t, r, "POST", "/api/bindings", tk, map[string]any{"segment_id": segID, "gateway_id": gwID}); w.Code != 201 {
		t.Fatalf("create binding: %d", w.Code)
	}

	// 不存在的网段建绑定 -> 409
	w = do(t, r, "POST", "/api/bindings", tk, map[string]any{"segment_id": 9999, "gateway_id": gwID})
	if w.Code != 409 {
		t.Fatalf("binding for missing segment should 409, got %d", w.Code)
	}

	// 删除网段 -> 绑定应被级联清理
	w = do(t, r, "DELETE", "/api/segments/"+itoa(segID), tk, nil)
	if w.Code != 204 {
		t.Fatalf("delete segment: %d", w.Code)
	}
	w = do(t, r, "GET", "/api/bindings?segment_id="+itoa(segID), tk, nil)
	if w.Code != 200 || !bytes.Contains(w.Body.Bytes(), []byte(`"items":[]`)) {
		t.Fatalf("bindings after delete not empty: %d %s", w.Code, w.Body.String())
	}
}

func TestActivateBindingKeepsExistingGatewaysEnabled(t *testing.T) {
	gdb, err := db.Open(&config.AppConfig{DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sqlDB, _ := gdb.DB(); _ = sqlDB.Close() })
	seg := models.Segment{Name: "10.0.0.0/8", Cidr: "10.0.0.0/8", Netmask: "255.0.0.0", Enabled: true}
	oldGateway := models.Gateway{Name: "旧", GatewayIP: "192.168.1.2", Enabled: true}
	newGateway := models.Gateway{Name: "新", GatewayIP: "192.168.1.3", Enabled: true}
	_ = gdb.Create(&seg).Error
	_ = gdb.Create(&oldGateway).Error
	_ = gdb.Create(&newGateway).Error
	_ = gdb.Create(&models.Binding{SegmentID: seg.ID, GatewayID: oldGateway.ID, IsActive: true, Enabled: true}).Error
	_ = gdb.Create(&models.Binding{SegmentID: seg.ID, GatewayID: newGateway.ID, Enabled: true}).Error

	s := &Server{db: gdb, eng: sync.New(gdb), elevated: true}
	if err := s.activateBinding(seg.ID, newGateway.ID); err != nil {
		t.Fatal(err)
	}
	var bindings []models.Binding
	if err := gdb.Where("segment_id = ?", seg.ID).Find(&bindings).Error; err != nil {
		t.Fatal(err)
	}
	if len(bindings) != 2 || !bindings[0].Enabled || !bindings[1].Enabled {
		t.Fatalf("unexpected bindings after switch: %+v", bindings)
	}
}
