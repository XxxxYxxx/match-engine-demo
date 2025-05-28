// package main

// // Order 订单结构体
// type Order struct {
// 	OrderID   string  `json:"order_id"`
// 	UserID    int     `json:"user_id"`    // 改为 int
// 	OrderType string  `json:"order_type"` // BID or ASK
// 	OrderKind string  `json:"order_kind"` // LIMIT or MARKET
// 	Price     float64 `json:"price"`
// 	Amount    float64 `json:"amount"`
// 	Timestamp int64   `json:"timestamp"`
// }

// // Trade 成交结构体
// type Trade struct {
// 	TradeID    string
// 	BidOrderID string
// 	AskOrderID string
// 	Price      float64
// 	Amount     float64
// }

package main

import (
	"github.com/shopspring/decimal"
)

// Order 订单结构体
type Order struct {
	OrderID   string          `json:"order_id"`
	UserID    int             `json:"user_id"`
	OrderType string          `json:"order_type"` // BID 或 ASK
	OrderKind string          `json:"order_kind"` // LIMIT 或 MARKET
	Price     decimal.Decimal `json:"price"`
	Amount    decimal.Decimal `json:"amount"`
	Timestamp int64           `json:"timestamp"` // Unix 时间戳（秒）
}

// Trade 交易结构体
type Trade struct {
	TradeID    string          `json:"trade_id"`
	BidOrderID string          `json:"bid_order_id"`
	AskOrderID string          `json:"ask_order_id"`
	Price      decimal.Decimal `json:"price"`
	Amount     decimal.Decimal `json:"amount"`
}

type OrderBookLevel struct {
	Price  decimal.Decimal `json:"price"`  // 价格
	Amount decimal.Decimal `json:"amount"` // 数量
}

// 盘口结构体
type OrderBookSnapshot struct {
	Pair string           `json:"pair"`
	Bids []OrderBookLevel `json:"bids"`
	Asks []OrderBookLevel `json:"asks"`
}
