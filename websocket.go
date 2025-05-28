package main

import (
	"net/http"
	"sort"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/shopspring/decimal"
)

// WebSocket 客户端管理
var (
	upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}
	wsClients   = make(map[*websocket.Conn]bool)
	wsClientsMu sync.Mutex
)

// 推送盘口信息到所有 WebSocket 客户端
func broadcastOrderBook(snapshot OrderBookSnapshot) {
	wsClientsMu.Lock()
	defer wsClientsMu.Unlock()
	for conn := range wsClients {
		err := conn.WriteJSON(snapshot)
		if err != nil {
			conn.Close()
			delete(wsClients, conn)
		}
	}
}

// WebSocket handler
func wsOrderBookHandler(rc *RedisClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		wsClientsMu.Lock()
		wsClients[conn] = true
		wsClientsMu.Unlock()
		// 可选：初次连接时推送一次盘口
		// go func() {
		// 	snapshot := getOrderBookSnapshot(rc, "BTCUSDT") // 示例
		// 	conn.WriteJSON(snapshot)
		// }()
	}
}

// 从 Redis 获取订单簿并聚合
func getOrderBookSnapshot(rc *RedisClient, pair string) OrderBookSnapshot {
	bidOrders, _ := rc.GetAllOrders("bids:" + pair)
	askOrders, _ := rc.GetAllOrders("asks:" + pair)

	bidMap := make(map[string]decimal.Decimal)
	askMap := make(map[string]decimal.Decimal)

	// 聚合买单
	for _, o := range bidOrders {
		priceStr := o.Price.String()
		bidMap[priceStr] = bidMap[priceStr].Add(o.Amount)
	}
	// 聚合卖单
	for _, o := range askOrders {
		priceStr := o.Price.String()
		askMap[priceStr] = askMap[priceStr].Add(o.Amount)
	}

	// 转为切片
	var bids []OrderBookLevel
	for priceStr, amount := range bidMap {
		price, _ := decimal.NewFromString(priceStr)
		bids = append(bids, OrderBookLevel{Price: price, Amount: amount})
	}
	var asks []OrderBookLevel
	for priceStr, amount := range askMap {
		price, _ := decimal.NewFromString(priceStr)
		asks = append(asks, OrderBookLevel{Price: price, Amount: amount})
	}

	// 买单按价格降序
	sort.Slice(bids, func(i, j int) bool {
		return bids[i].Price.GreaterThan(bids[j].Price)
	})
	// 卖单按价格升序
	sort.Slice(asks, func(i, j int) bool {
		return asks[i].Price.LessThan(asks[j].Price)
	})

	return OrderBookSnapshot{
		Pair: pair,
		Bids: bids,
		Asks: asks,
	}
}
