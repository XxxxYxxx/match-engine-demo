package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// Order 订单结构体，与主程序保持一致
type Order struct {
	OrderID   string          `json:"order_id"`
	UserID    int             `json:"user_id"`
	OrderType string          `json:"order_type"`
	OrderKind string          `json:"order_kind"` // LIMIT or MARKET
	Price     decimal.Decimal `json:"price"`
	Amount    decimal.Decimal `json:"amount"`
	Timestamp int64           `json:"timestamp"`
}

type PostgresClient struct {
	db *gorm.DB
}

const pgConnStr = "host=localhost port=5432 user=admin password=xtCnuCTpu5YG9WNP dbname=crypto_exchange sslmode=disable"

// NewPostgresClient 初始化 PostgreSQL 客户端
func NewPostgresClient() (*PostgresClient, error) {
	db, err := gorm.Open(postgres.Open(pgConnStr), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("无法连接到 PostgreSQL: %v", err)
	}
	return &PostgresClient{db: db}, nil
}

// Close 关闭数据库连接
func (pc *PostgresClient) Close() {
	if sqlDB, err := pc.db.DB(); err == nil {
		sqlDB.Close()
	}
}

// GetValidUserIDs 获取 users 表中的所有 user_id
func (pc *PostgresClient) GetValidUserIDs() ([]int, error) {
	var userIDs []int
	if err := pc.db.Table("users").Select("user_id").Find(&userIDs).Error; err != nil {
		return nil, fmt.Errorf("查询用户失败: %v", err)
	}
	if len(userIDs) == 0 {
		return nil, fmt.Errorf("users 表为空")
	}
	return userIDs, nil
}

// generateRandomOrder 生成随机订单，支持限价和市价订单
func generateRandomOrder(userIDs []int) Order {
	orderTypes := []string{"BID", "ASK"}
	orderKinds := []string{"LIMIT", "MARKET"}

	// 随机生成价格和数量
	price := decimal.NewFromFloat(40000 + rand.Float64()*1000).Round(8)
	amount := decimal.NewFromFloat(0.01 + rand.Float64()*0.99).Round(8)
	order := Order{
		OrderID:   uuid.New().String(),
		UserID:    userIDs[rand.Intn(len(userIDs))],
		OrderType: orderTypes[rand.Intn(len(orderTypes))],
		OrderKind: orderKinds[rand.Intn(len(orderKinds))],
		Price:     price,
		Amount:    amount,
		Timestamp: time.Now().UTC().Unix(), // 秒级时间戳
	}

	// 市价订单价格设为 0
	if order.OrderKind == "MARKET" {
		order.Price = decimal.Zero
	}

	return order
}

// sendOrder 发送订单到 HTTP 接口
func sendOrder(order Order) error {
	orderJSON, err := json.Marshal(order)
	if err != nil {
		return fmt.Errorf("序列化订单失败: %v", err)
	}

	resp, err := http.Post("http://localhost:8080/orders", "application/json", bytes.NewBuffer(orderJSON))
	if err != nil {
		return fmt.Errorf("发送 HTTP 请求失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP 请求失败，状态码: %d", resp.StatusCode)
	}

	var result map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("解析响应失败: %v", err)
	}
	log.Printf("订单提交成功: %s", result["order_id"])
	return nil
}

func main() {
	// 初始化随机数种子
	rand.Seed(time.Now().UnixNano())

	// 连接 PostgreSQL
	pc, err := NewPostgresClient()
	if err != nil {
		log.Fatal(err)
	}
	defer pc.Close()

	// 获取有效 user_id
	userIDs, err := pc.GetValidUserIDs()
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("有效用户 ID: %v", userIDs)

	// 每 3 秒发送一个随机订单
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		order := generateRandomOrder(userIDs)
		log.Printf("生成订单: %+v", order)
		if err := sendOrder(order); err != nil {
			log.Printf("发送订单失败: %v", err)
			continue
		}
	}
}
