package main

// Order 订单结构体
type Order struct {
	OrderID   string  `json:"order_id"`
	UserID    int     `json:"user_id"`    // 改为 int
	OrderType string  `json:"order_type"` // BID or ASK
	Price     float64 `json:"price"`
	Amount    float64 `json:"amount"`
	Timestamp int64   `json:"timestamp"`
}

// Trade 成交结构体
type Trade struct {
	TradeID    string
	BidOrderID string
	AskOrderID string
	Price      float64
	Amount     float64
}
