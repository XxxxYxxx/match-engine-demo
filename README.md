# Orderbook 撮合引擎

基于 Go 语言实现的撮合引擎demo，支持市价单和限价单的撮合。订单撮合采用 Redis 作为撮合簿缓存，PostgreSQL 作为持久化存储。

## 主要功能

- 支持市价单与限价单撮合
- 支持买单（BID）和卖单（ASK）
- 撮合遵循价格优先、时间优先（FIFO）原则
- 订单状态自动更新（FILLED、PARTIALLY_FILLED、CLOSE）
- 交易撮合结果实时推送
- 订单簿与撮合结果持久化到 PostgreSQL

## 技术栈

- Go 1.18+
- Redis
- PostgreSQL
- GORM
- github.com/shopspring/decimal
- github.com/google/uuid

## 快速开始

1. **克隆项目**
   ```bash
   git clone https://github.com/XxxxYxxx/orderbook.git
   cd orderbook
   ```

2. **配置数据库和 Redis**
   - 启动 Redis 和 PostgreSQL 服务
   - 配置数据库连接参数（见代码中的 `PostgresClient` 和 `RedisClient`）

3. **安装依赖**
   ```bash
   go mod tidy
   ```

4. **编译并运行**
   ```bash
   go build -o orderbook
   ./orderbook
   ```

## 主要接口说明

- `matchOrdersMarket(rc, pc, pair, newOrder)`  
  市价单撮合，自动与对手盘最佳价格订单成交，未成交部分自动取消。

- `matchOrdersPriceLimit(rc, pc, pair, newOrder)`  
  限价单撮合，价格匹配时成交，未成交部分保留在订单簿。

- `AddOrderToBook(order, pair)`  
  将订单添加到 Redis 订单簿。

- `RemoveOrder(key, orderJSON)`  
  从订单簿移除订单。

- `SaveTrade(trade)`  
  保存成交记录到数据库。

## 订单与交易结构

```go
type Order struct {
    OrderID   string
    OrderType string // "BID" or "ASK"
    Price     decimal.Decimal
    Amount    decimal.Decimal
    Timestamp int64
    Status    string // "OPEN", "FILLED", "PARTIALLY_FILLED", "CLOSE"
}

type Trade struct {
    TradeID    string
    BidOrderID string
    AskOrderID string
    Price      decimal.Decimal
    Amount     decimal.Decimal
}
```

## 贡献

欢迎提交 issue 和 PR 进行改进！

## License

MIT
