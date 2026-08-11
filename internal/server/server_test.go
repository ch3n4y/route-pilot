package server

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"route-manager/internal/config"
	"route-manager/internal/db"
	"route-manager/internal/sync"
)

func do(t *testing.T, eng *gin.Engine, method, path, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	eng.ServeHTTP(w, req)
	return w
}

func newTestServer(t *testing.T) (*gin.Engine, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	gdb, err := db.Open(&config.AppConfig{DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sqlDB, _ := gdb.DB(); sqlDB.Close() })
	eng := sync.New(gdb)
	r := New(gdb, eng, &config.AppConfig{Host: "0.0.0.0", DataDir: t.TempDir()}, "test", true, nil, nil)
	return r, "tok"
}

func TestAuthAndCRUDFlow(t *testing.T) {
	r, _ := newTestServer(t)

	// 1. setup
	w := do(t, r, "GET", "/api/setup/status", "", nil)
	if w.Code != 200 || !bytes.Contains(w.Body.Bytes(), []byte("true")) {
		t.Fatalf("setup/status: %d %s", w.Code, w.Body.String())
	}
	w = do(t, r, "POST", "/api/setup", "", map[string]string{"password": "secret123"})
	if w.Code != 201 {
		t.Fatalf("setup: %d %s", w.Code, w.Body.String())
	}
	var setupResp struct {
		Token string `json:"token"`
	}
	json.Unmarshal(w.Body.Bytes(), &setupResp)
	token := setupResp.Token
	if token == "" {
		t.Fatal("no token from setup")
	}

	// 2. unauthenticated blocked
	w = do(t, r, "GET", "/api/segments", "", nil)
	if w.Code != 401 {
		t.Fatalf("segments without auth: %d", w.Code)
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
	w = do(t, r, "POST", "/api/segments", token, map[string]any{"name": "重叠", "cidr": "10.5.0.0/16"})
	if w.Code != 422 {
		t.Fatalf("overlap should 422: %d %s", w.Code, w.Body.String())
	}

	// 4. create gateway + invalid ip rejected
	w = do(t, r, "POST", "/api/gateways", token, map[string]any{"name": "GW-LAN", "gateway_ip": "192.168.1.2"})
	if w.Code != 201 {
		t.Fatalf("create gateway: %d %s", w.Code, w.Body.String())
	}
	var gwResp struct {
		Item struct{ ID uint `json:"id"` }
	}
	json.Unmarshal(w.Body.Bytes(), &gwResp)
	gwID := gwResp.Item.ID
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

	// 7. login wrong + right
	w = do(t, r, "POST", "/api/login", "", map[string]string{"password": "wrong"})
	if w.Code != 401 {
		t.Fatalf("wrong password should 401: %d", w.Code)
	}
	w = do(t, r, "POST", "/api/login", "", map[string]string{"password": "secret123"})
	if w.Code != 200 {
		t.Fatalf("login: %d %s", w.Code, w.Body.String())
	}

	// 8. change password
	w = do(t, r, "PUT", "/api/settings/password", token, map[string]string{"old_password": "secret123", "new_password": "newpass"})
	if w.Code != 204 {
		t.Fatalf("change password: %d %s", w.Code, w.Body.String())
	}
	w = do(t, r, "POST", "/api/login", "", map[string]string{"password": "newpass"})
	if w.Code != 200 {
		t.Fatalf("login after change: %d", w.Code)
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
	w := do(t, r, "POST", "/api/setup", "", map[string]string{"password": "secret123"})
	var s struct {
		Token string `json:"token"`
	}
	json.Unmarshal(w.Body.Bytes(), &s)
	tk := s.Token

	// 建网段+网关+绑定
	w = do(t, r, "POST", "/api/segments", tk, map[string]any{"name": "s", "cidr": "10.0.0.0/8"})
	json.Unmarshal(w.Body.Bytes(), &struct{ Item struct{ ID uint } }{})
	var segResp struct{ Item struct{ ID uint } }
	json.Unmarshal(w.Body.Bytes(), &segResp)
	segID := segResp.Item.ID
	w = do(t, r, "POST", "/api/gateways", tk, map[string]any{"name": "g", "gateway_ip": "192.168.1.2"})
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
