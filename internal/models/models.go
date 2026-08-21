package models

import "time"

type Segment struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	Name        string    `gorm:"not null" json:"-"`
	Cidr        string    `gorm:"not null;uniqueIndex" json:"cidr"` // 规范化网络地址 10.0.0.0/8
	Netmask     string    `gorm:"not null" json:"netmask"`
	Metric      int       `gorm:"default:1" json:"metric"` // 基础跃点；被更具体网段覆盖时由引擎自动提升（有效跃点）
	Description string    `gorm:"default:''" json:"description"`
	Enabled     bool      `json:"enabled"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Gateway struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	Name        string    `gorm:"not null" json:"name"`
	GatewayIP   string    `gorm:"not null" json:"gateway_ip"`
	Interface   string    `gorm:"default:''" json:"interface"`
	IfIndex     int       `gorm:"default:0;column:ifindex" json:"ifindex"`
	Description string    `gorm:"default:''" json:"description"`
	Enabled     bool      `json:"enabled"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Binding struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	SegmentID uint      `gorm:"not null;index:idx_seg_gw,unique;constraint:OnUpdate:CASCADE,OnDelete:CASCADE" json:"segment_id"`
	GatewayID uint      `gorm:"not null;index:idx_seg_gw,unique;constraint:OnUpdate:CASCADE,OnDelete:CASCADE" json:"gateway_id"`
	IsActive  bool      `gorm:"default:false" json:"is_active"`
	Position  int       `gorm:"default:0" json:"position"`
	Enabled   bool      `json:"enabled"`
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
	IfIndex    int       `gorm:"default:0;column:ifindex" json:"ifindex"`
	Status     string    `gorm:"default:'OK'" json:"status"` // OK|MISSING|CONFLICT|ERROR
	LastError  string    `gorm:"default:''" json:"last_error"`
	LastSyncAt time.Time `json:"last_sync_at"`
}
