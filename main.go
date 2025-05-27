package main

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	_ "net/http/pprof"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/shopspring/decimal"
)

func main() {
	// 初始化 Redis
	rc := NewRedisClient()
	defer rc.Close()

	// 初始化 PostgreSQL
	pc, err := NewPostgresClient()
	if err != nil {
		log.Fatal(err)
	}
	defer pc.Close()

	// 初始化订单簿
	if err := rc.InitOrderBook("BTC_USDT"); err != nil {
		log.Fatal("初始化订单簿失败:", err)
	}

	// Redis 订阅
	go func() {
		log.Println("启动 incoming_orders 订阅")
		rc.SubscribeOrders("incoming_orders", func(order Order) {
			log.Printf("处理订单: %+v", order)
			if err := pc.SaveOrder(order); err != nil {
				log.Printf("保存订单到数据库失败: %v", err)
				return
			}
			var err error
			if order.OrderKind == "MARKET" {
				err = matchOrdersMarket(rc, pc, "BTC_USDT", order)
			} else {
				err = matchOrdersPriceLimit(rc, pc, "BTC_USDT", order)
			}
			if err != nil {
				log.Printf("撮合订单失败: %v", err)
			}
		})
	}()

	// Redis 成交订阅
	// go func() {
	// 	log.Println("启动 completed_trades 订阅")
	// 	rc.client.Subscribe(rc.ctx, "completed_trades").Channel()
	// 	pubsub := rc.client.Subscribe(rc.ctx, "completed_trades")
	// 	for msg := range pubsub.Channel() {
	// 		log.Printf("收到成交消息: %s", msg.Payload)
	// 		var trade Trade
	// 		if err := json.Unmarshal([]byte(msg.Payload), &trade); err != nil {
	// 			log.Printf("解析成交失败: %v, 消息: %s", err, msg.Payload)
	// 			continue
	// 		}
	// 		log.Printf("解析成交成功: %+v", trade)
	// 		// 可添加处理逻辑，如通知前端、记录日志
	// 	}
	// }()

	// 启动 HTTP 服务器
	go func() {
		router := mux.NewRouter()
		router.HandleFunc("/orders", handleOrder(pc, rc)).Methods("POST")
		log.Println("HTTP 服务器启动在 :8080")
		if err := http.ListenAndServe(":8080", router); err != nil {
			log.Fatal("HTTP 服务器启动失败:", err)
		}
	}()

	go func() {
		log.Println(http.ListenAndServe("localhost:6060", nil))
	}()

	select {} // 保持程序运行
}

// handleOrder 处理 POST /orders 请求
func handleOrder(pc *PostgresClient, rc *RedisClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var order Order
		if err := json.NewDecoder(r.Body).Decode(&order); err != nil {
			http.Error(w, "无效的订单格式", http.StatusBadRequest)
			log.Printf("解析订单失败: %v", err)
			return
		}

		// 验证订单字段
		if order.OrderID == "" {
			order.OrderID = uuid.New().String()
		}
		if order.OrderType != "BID" && order.OrderType != "ASK" {
			http.Error(w, "无效的订单类型，必须是 BID 或 ASK", http.StatusBadRequest)
			return
		}
		if order.OrderKind != "LIMIT" && order.OrderKind != "MARKET" {
			http.Error(w, "无效的订单种类，必须是 LIMIT 或 MARKET", http.StatusBadRequest)
			return
		}
		if order.OrderKind == "LIMIT" && order.Price.LessThanOrEqual(decimal.Zero) {
			http.Error(w, "限价订单价格必须大于 0", http.StatusBadRequest)
			return
		}
		if order.Amount.LessThanOrEqual(decimal.Zero) {
			http.Error(w, "订单数量必须大于 0", http.StatusBadRequest)
			return
		}
		if order.Timestamp == 0 {
			order.Timestamp = time.Now().Unix()
		}

		// 验证 user_id 存在
		if err := pc.ValidateUser(order.UserID); err != nil {
			http.Error(w, "用户不存在", http.StatusBadRequest)
			log.Printf("用户验证失败: %v", err)
			return
		}

		// 发布订单到 Redis
		if err := rc.SubmitOrder(order); err != nil {
			http.Error(w, "提交订单失败", http.StatusInternalServerError)
			log.Printf("提交订单到 Redis 失败: %v", err)
			return
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"message": "订单提交成功", "order_id": order.OrderID})
	}
}
