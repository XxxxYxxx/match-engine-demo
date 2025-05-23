package main

import (
	"fmt"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

const pgConnStr = "host=localhost port=5432 user=admin password=xtCnuCTpu5YG9WNP dbname=crypto_exchange sslmode=disable"

// PostgresClient 封装GORM客户端
type PostgresClient struct {
	db *gorm.DB
}

// NewPostgresClient 初始化GORM客户端
func NewPostgresClient() (*PostgresClient, error) {
	db, err := gorm.Open(postgres.Open(pgConnStr), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("无法连接到 PostgreSQL: %v", err)
	}

	// 自动迁移数据库结构
	if err := db.AutoMigrate(&OrderModel{}, &TradeModel{}); err != nil {
		return nil, fmt.Errorf("自动迁移失败: %v", err)
	}

	return &PostgresClient{db: db}, nil
}

// Close 关闭GORM客户端
func (pc *PostgresClient) Close() {
	if sqlDB, err := pc.db.DB(); err == nil {
		sqlDB.Close()
	}
}

// OrderModel 映射到orders表
type OrderModel struct {
	OrderID   string `gorm:"primaryKey;type:uuid"`
	UserID    int    `gorm:"type:integer;foreignKey:UserID;references:users(user_id)"` // 改为整型并添加外键
	Pair      string `gorm:"type:varchar(20);default:BTC_USDT"`
	OrderType string `gorm:"type:varchar(4)"` // 去掉CHECK约束，由应用层验证
	Price     float64
	Amount    float64
	Status    string `gorm:"type:varchar(20);default:OPEN"`
	Timestamp int64  `gorm:"timestamp"`
}

// TradeModel 映射到trades表
type TradeModel struct {
	TradeID    string `gorm:"primaryKey;type:uuid"`
	BidOrderID string `gorm:"type:uuid"`
	AskOrderID string `gorm:"type:uuid"`
	Price      float64
	Amount     float64
	Timestamp  int64 `gorm:"timestamp"`
}

// TableName 指定OrderModel的表名
func (OrderModel) TableName() string {
	return "orders"
}

// TableName 指定TradeModel的表名
func (TradeModel) TableName() string {
	return "trades"
}

// SaveOrder 保存订单到数据库
func (pc *PostgresClient) SaveOrder(order Order) error {
	orderModel := OrderModel{
		OrderID:   order.OrderID,
		UserID:    order.UserID,
		Pair:      "BTC_USDT",
		OrderType: order.OrderType,
		Price:     order.Price,
		Amount:    order.Amount,
		Status:    "OPEN",
		Timestamp: order.Timestamp,
	}
	return pc.db.Create(&orderModel).Error
}

// SaveTrade 保存成交到数据库
func (pc *PostgresClient) SaveTrade(trade Trade) error {
	tradeModel := TradeModel{
		TradeID:    trade.TradeID,
		BidOrderID: trade.BidOrderID,
		AskOrderID: trade.AskOrderID,
		Price:      trade.Price,
		Amount:     trade.Amount,
		Timestamp:  time.Now().Unix(),
	}
	return pc.db.Create(&tradeModel).Error
}

// ValidateUser 检查 user_id 是否存在于 users 表
func (pc *PostgresClient) ValidateUser(userID int) error {
	var count int64
	if err := pc.db.Table("users").Where("user_id = ?", userID).Count(&count).Error; err != nil {
		return fmt.Errorf("查询用户失败: %v", err)
	}
	if count == 0 {
		return fmt.Errorf("用户 %d 不存在", userID)
	}
	return nil
}
