package models

import "time"

type Segment struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	Name        string    `gorm:"not null" json:"name"`
	Cidr        string    `gorm:"not null;uniqueIndex" json:"cidr"` // 规范化网络地址 10.0.0.0/8
	Netmask     string    `gorm:"not null" json:"netmask"`
	Description string    `gorm:"default:''" json:"description"`
	Enabled     bool      `gorm:"default:true" json:"enabled"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Gateway struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	Name        string    `gorm:"not null" json:"name"`
	GatewayIP   string    `gorm:"not null" json:"gateway_ip"`
	Interface   string    `gorm:"default:''" json:"interface"`
	IfIndex     int       `gorm:"default:0" json:"ifindex"`
	Metric      int       `gorm:"default:1" json:"metric"`
	Description string    `gorm:"default:''" json:"description"`
	Enabled     bool      `gorm:"default:true" json:"enabled"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Binding struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	SegmentID uint      `gorm:"not null;index:idx_seg_gw,unique" json:"segment_id"`
	GatewayID uint      `gorm:"not null;index:idx_seg_gw,unique" json:"gateway_id"`
	IsActive  bool      `gorm:"default:false" json:"is_active"`
	Position  int       `gorm:"default:0" json:"position"`
	Enabled   bool      `gorm:"default:true" json:"enabled"`
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
	IfIndex    int       `gorm:"default:0" json:"ifindex"`
	Status     string    `gorm:"default:'OK'" json:"status"` // OK|MISSING|CONFLICT|ERROR
	LastError  string    `gorm:"default:''" json:"last_error"`
	LastSyncAt time.Time `json:"last_sync_at"`
}
