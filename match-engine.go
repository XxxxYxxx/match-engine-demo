package main

import (
	"encoding/json"

	"github.com/google/uuid"
)

func matchOrders(rc *RedisClient, pc *PostgresClient, pair string, order Order) error {
	oppositeKey := "asks:" + pair
	orderKey := "bids:" + pair
	if order.OrderType == "ASK" {
		oppositeKey = "bids:" + pair
		orderKey = "asks:" + pair
	}

	remainingAmount := order.Amount
	for remainingAmount > 0 {
		// 获取对手方最佳订单
		bestOrder, bestPrice, err := rc.GetBestOrder(oppositeKey)
		if err != nil || bestOrder == nil {
			break
		}

		// 检查价格匹配
		if order.OrderType == "BID" && bestPrice > order.Price {
			break
		}
		if order.OrderType == "ASK" && bestPrice < order.Price {
			break
		}

		// 计算成交量
		tradeAmount := min(remainingAmount, bestOrder.Amount)
		tradePrice := bestPrice

		// 更新订单簿
		bestOrderJSON, _ := json.Marshal(bestOrder)
		if err := rc.RemoveOrder(oppositeKey, string(bestOrderJSON)); err != nil {
			return err
		}
		if bestOrder.Amount > tradeAmount {
			bestOrder.Amount -= tradeAmount
			rc.AddOrderToBook(*bestOrder, pair)
		}

		// 更新当前订单
		remainingAmount -= tradeAmount
		if remainingAmount > 0 {
			orderJSON, _ := json.Marshal(order)
			rc.RemoveOrder(orderKey, string(orderJSON))
			order.Amount = remainingAmount
			rc.AddOrderToBook(order, pair)
		} else {
			orderJSON, _ := json.Marshal(order)
			rc.RemoveOrder(orderKey, string(orderJSON))
		}

		// 记录成交
		trade := Trade{
			TradeID:    uuid.New().String(),
			BidOrderID: order.OrderID,
			AskOrderID: bestOrder.OrderID,
			Price:      tradePrice,
			Amount:     tradeAmount,
		}
		if order.OrderType == "ASK" {
			trade.BidOrderID, trade.AskOrderID = bestOrder.OrderID, order.OrderID
		}
		if err := pc.SaveTrade(trade); err != nil {
			return err
		}

		// 推送成交通知
		if err := rc.PublishTrade(trade); err != nil {
			return err
		}
	}

	return nil
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
