package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/go-redis/redis/v8"
	"github.com/shopspring/decimal"
)

const (
	defaultRedisAddr = "127.0.0.1:6380"
	defaultRedisDB   = 1
	pricePrecision   = 1e8   // 1e8，8 位小数
	maxTimestampDiff = 86400 // 最大时间差（秒）
)

// RedisClient Redis 客户端
type RedisClient struct {
	client *redis.Client
	ctx    context.Context
}

func getRedisAddr() string {
	addr := os.Getenv("REDIS_ADDR")
	addr = strings.TrimPrefix(addr, "http://")
	addr = strings.TrimPrefix(addr, "https://")
	if addr == "" {
		return defaultRedisAddr
	}
	return addr
}

func getRedisPassword() string {
	return os.Getenv("REDIS_PASSWORD")
}

func getRedisDB() int {
	dbStr := os.Getenv("REDIS_DB")
	if dbStr == "" {
		return defaultRedisDB
	}
	db, err := strconv.Atoi(dbStr)
	if err != nil {
		log.Printf("无效的 Redis 数据库编号，使用默认: %v", err)
		return defaultRedisDB
	}
	return db
}

func NewRedisClient() *RedisClient {
	addr := getRedisAddr()
	password := getRedisPassword()
	db := getRedisDB()
	log.Printf("连接 Redis 地址: %s, 数据库: %d", addr, db)
	rdb := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		log.Fatalf("Redis 连接失败: %v", err)
	}
	return &RedisClient{
		client: rdb,
		ctx:    context.Background(),
	}
}

func (rc *RedisClient) Close() {
	rc.client.Close()
}

func (rc *RedisClient) InitOrderBook(pair string) error {
	return rc.client.Del(rc.ctx, "bids:"+pair, "asks:"+pair).Err()
}

func (rc *RedisClient) SubmitOrder(order Order) error {
	orderJSON, err := json.Marshal(order)
	if err != nil {
		log.Printf("序列化订单失败: %v", err)
		return err
	}
	log.Printf("发布订单到通道 incoming_orders: %s", orderJSON)
	return rc.client.Publish(rc.ctx, "incoming_orders", orderJSON).Err()
}

func (rc *RedisClient) AddOrderToBook(order Order, pair string) error {
	redisKey := "bids:" + pair
	if order.OrderType == "ASK" {
		redisKey = "asks:" + pair
	}
	// 截断价格到 8 位小数
	price := order.Price.Round(8)
	if price.LessThanOrEqual(decimal.Zero) || price.GreaterThan(decimal.NewFromInt(1000000)) {
		log.Printf("无效价格: %v", price)
		return fmt.Errorf("价格超出范围: %v", price)
	}
	orderJSON, err := json.Marshal(order)
	if err != nil {
		log.Printf("序列化订单失败: %v", err)
		return err
	}
	priceScore := price.Mul(decimal.NewFromInt(pricePrecision)) // Price * 1e8
	log.Printf("添加订单到 %s, 原始价格: %v, 截断价格: %v, 时间戳: %v, 分值: %v",
		redisKey, order.Price, price, order.Timestamp, priceScore)
	return rc.client.ZAdd(rc.ctx, redisKey, &redis.Z{Score: priceScore.InexactFloat64(), Member: orderJSON}).Err()
}

func (rc *RedisClient) GetBestOrder(redisKey string) (*Order, decimal.Decimal, error) {
	bestOrders, err := rc.client.ZRangeWithScores(rc.ctx, redisKey, 0, 0).Result()
	if err != nil || len(bestOrders) == 0 {
		return nil, decimal.Zero, err
	}
	var bestOrder Order
	if err := json.Unmarshal([]byte(bestOrders[0].Member.(string)), &bestOrder); err != nil {
		log.Printf("解析最佳订单失败: %v", err)
		return nil, decimal.Zero, err
	}
	// 提取价格部分
	score := decimal.NewFromFloat(bestOrders[0].Score)
	bestPrice := score.Floor().Div(decimal.NewFromInt(pricePrecision))
	log.Printf("获取最佳订单: %+v, 分值: %v, 价格: %v", bestOrder, score, bestPrice)
	return &bestOrder, bestPrice, nil
}

func (rc *RedisClient) RemoveOrder(redisKey string, orderJSON string) error {
	return rc.client.ZRem(rc.ctx, redisKey, orderJSON).Err()
}

func (rc *RedisClient) GetOrdersByPrice(redisKey string, price decimal.Decimal) ([]Order, error) {
	// 截断价格到 8 位小数
	priceScore := price.Mul(decimal.NewFromInt(pricePrecision))
	// 分值范围：priceScore 到 priceScore + maxTimestampDiff/1e8

	results, err := rc.client.ZRangeByScoreWithScores(rc.ctx, redisKey, &redis.ZRangeBy{
		Min:    priceScore.String(),
		Max:    priceScore.String(),
		Offset: 0,
		Count:  -1,
	}).Result()
	if err != nil {
		log.Printf("获取同价订单失败: %v", err)
		return nil, err
	}

	orders := make([]Order, 0, len(results))
	for _, result := range results {
		var order Order
		if err := json.Unmarshal([]byte(result.Member.(string)), &order); err != nil {
			log.Printf("解析订单失败: %v", err)
			continue
		}
		// 验证价格匹配
		roundedPrice := order.Price.Round(8)
		if roundedPrice.Equal(price) {
			orders = append(orders, order)
		}
	}

	return orders, nil
}

func (rc *RedisClient) PublishTrade(trade Trade) error {
	tradeJSON, err := json.Marshal(trade)
	if err != nil {
		log.Printf("序列化交易失败: %v", err)
		return err
	}
	log.Printf("发布交易到通道 completed_trades: %s", tradeJSON)
	err = rc.client.Publish(rc.ctx, "completed_trades", tradeJSON).Err()
	if err != nil {
		log.Printf("发布交易到 completed_trades 失败: %v", err)
		return err
	}
	return nil
}

func (rc *RedisClient) SubscribeOrders(channel string, handler func(Order)) {
	pubsub := rc.client.Subscribe(rc.ctx, channel)
	log.Printf("订阅通道: %s", channel)
	for msg := range pubsub.Channel() {
		log.Printf("收到消息: %s", msg.Payload)
		var order Order
		if err := json.Unmarshal([]byte(msg.Payload), &order); err != nil {
			log.Printf("解析订单失败: %v, 消息: %s", err, msg.Payload)
			continue
		}
		log.Printf("解析订单成功: %+v", order)
		handler(order)
	}
}

func (rc *RedisClient) GetAllOrders(redisKey string) ([]Order, error) {
	results, err := rc.client.ZRangeWithScores(rc.ctx, redisKey, 0, -1).Result()
	if err != nil {
		log.Printf("获取所有订单失败: %v", err)
		return nil, err
	}

	orders := make([]Order, 0, len(results))
	for _, result := range results {
		var order Order
		if err := json.Unmarshal([]byte(result.Member.(string)), &order); err != nil {
			log.Printf("解析订单失败: %v", err)
			continue
		}
		orders = append(orders, order)
	}

	return orders, nil
}
