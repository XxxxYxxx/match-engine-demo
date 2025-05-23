package main

import (
	"context"
	"encoding/json"
	"log"

	"github.com/go-redis/redis/v8"
)

const (
	redisAddr      = "127.0.0.1:6380"
	pricePrecision = 100000000 // 价格放大10^8
)

// RedisClient 封装Redis客户端
type RedisClient struct {
	client *redis.Client
	ctx    context.Context
}

// NewRedisClient 初始化Redis客户端
func NewRedisClient() *RedisClient {
	rdb := redis.NewClient(&redis.Options{Addr: redisAddr, Password: "XxxxYxxx", DB: 0})
	return &RedisClient{
		client: rdb,
		ctx:    context.Background(),
	}
}

// Close 关闭Redis客户端
func (rc *RedisClient) Close() {
	rc.client.Close()
}

// InitOrderBook 初始化订单簿
func (rc *RedisClient) InitOrderBook(pair string) error {
	return rc.client.Del(rc.ctx, "bids:"+pair, "asks:"+pair).Err()
}

// SubmitOrder 提交订单到Redis
func (rc *RedisClient) SubmitOrder(order Order) error {
	orderJSON, err := json.Marshal(order)
	if err != nil {
		return err
	}
	return rc.client.Publish(rc.ctx, "incoming_orders", orderJSON).Err()
}

// AddOrderToBook 添加订单到订单簿
func (rc *RedisClient) AddOrderToBook(order Order, pair string) error {
	redisKey := "bids:" + pair
	if order.OrderType == "ASK" {
		redisKey = "asks:" + pair
	}
	orderJSON, err := json.Marshal(order)
	if err != nil {
		return err
	}
	score := order.Price * pricePrecision
	return rc.client.ZAdd(rc.ctx, redisKey, &redis.Z{Score: score, Member: orderJSON}).Err()
}

// GetBestOrder 获取对手方最佳订单
func (rc *RedisClient) GetBestOrder(redisKey string) (*Order, float64, error) {
	bestOrders, err := rc.client.ZRangeWithScores(rc.ctx, redisKey, 0, 0).Result()
	if err != nil || len(bestOrders) == 0 {
		return nil, 0, err
	}
	var bestOrder Order
	if err := json.Unmarshal([]byte(bestOrders[0].Member.(string)), &bestOrder); err != nil {
		return nil, 0, err
	}
	bestPrice := bestOrders[0].Score / pricePrecision
	return &bestOrder, bestPrice, nil
}

// RemoveOrder 从订单簿移除订单
func (rc *RedisClient) RemoveOrder(redisKey string, orderJSON string) error {
	return rc.client.ZRem(rc.ctx, redisKey, orderJSON).Err()
}

// PublishTrade 推送成交记录
func (rc *RedisClient) PublishTrade(trade Trade) error {
	tradeJSON, err := json.Marshal(trade)
	if err != nil {
		return err
	}
	return rc.client.Publish(rc.ctx, "completed_trades", tradeJSON).Err()
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
