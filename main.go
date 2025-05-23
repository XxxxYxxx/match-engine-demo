package main

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
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

	// 启动撮合引擎
	go func() {
		rc.SubscribeOrders("incoming_orders", func(order Order) {
			if err := pc.SaveOrder(order); err != nil {
				log.Printf("保存订单到数据库失败: %v", err)
				return
			}
			if err := rc.AddOrderToBook(order, "BTC_USDT"); err != nil {
				log.Printf("添加到 Redis 失败: %v", err)
				return
			}
			if err := matchOrders(rc, pc, "BTC_USDT", order); err != nil {
				log.Printf("撮合订单失败: %v", err)
			}
		})
	}()

	// 启动 HTTP 服务器
	go func() {
		router := mux.NewRouter()
		router.HandleFunc("/orders", handleOrder(pc, rc)).Methods("POST")
		log.Println("HTTP 服务器启动在 :8080")
		if err := http.ListenAndServe(":8080", router); err != nil {
			log.Fatal("HTTP 服务器启动失败:", err)
		}
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
		if order.Price <= 0 || order.Amount <= 0 {
			http.Error(w, "价格或数量必须大于 0", http.StatusBadRequest)
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
