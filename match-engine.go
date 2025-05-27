package main

import (
	"encoding/json"
	"log"
	"sort"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// matchOrdersMarket 撮合市价订单
func matchOrdersMarket(rc *RedisClient, pc *PostgresClient, pair string, newOrder Order) error {
	oppositeKey := "asks:" + pair
	if newOrder.OrderType == "ASK" {
		oppositeKey = "bids:" + pair
	}

	remainingAmount := newOrder.Amount
	originalOrder := newOrder

	// 使用 GORM 事务确保一致性
	err := pc.db.Transaction(func(tx *gorm.DB) error {
		for remainingAmount.GreaterThan(decimal.Zero) {
			// 获取对手盘最佳订单
			bestOrder, bestPrice, err := rc.GetBestOrder(oppositeKey)
			if err != nil {
				log.Printf("获取最佳订单失败: %v", err)
				return err
			}
			if bestOrder == nil {
				break // 无可撮合订单
			}

			// 获取同价格的所有订单，确保 FIFO
			orders, err := rc.GetOrdersByPrice(oppositeKey, bestPrice)
			if err != nil {
				log.Printf("获取同价订单失败: %v", err)
				return err
			}

			if len(orders) == 0 {
				break
			}

			// 按时间戳升序排序（FIFO）
			sort.Slice(orders, func(i, j int) bool {
				return orders[i].Timestamp < orders[j].Timestamp
			})

			// 撮合订单
			for _, matchOrder := range orders {
				if remainingAmount.LessThanOrEqual(decimal.Zero) {
					break
				}

				// 计算撮合金额
				matchAmount := min(remainingAmount, matchOrder.Amount)
				tradePrice := matchOrder.Price

				// 创建交易记录
				trade := Trade{
					TradeID:    uuid.New().String(),
					BidOrderID: newOrder.OrderID,
					AskOrderID: matchOrder.OrderID,
					Price:      tradePrice,
					Amount:     matchAmount,
				}
				if newOrder.OrderType == "ASK" {
					trade.BidOrderID, trade.AskOrderID = matchOrder.OrderID, newOrder.OrderID
				}

				// 保存交易
				if err := pc.SaveTrade(trade); err != nil {
					log.Printf("保存交易失败: %v", err)
					return err
				}
				if err := rc.PublishTrade(trade); err != nil {
					log.Printf("发布交易失败: %v", err)
					return err
				}

				// 更新订单
				remainingAmount = remainingAmount.Sub(matchAmount)

				// 从 Redis 移除匹配订单
				matchOrderJSON, err := json.Marshal(matchOrder)
				if err != nil {
					log.Printf("序列化匹配订单失败: %v", err)
					return err
				}
				if err := rc.RemoveOrder(oppositeKey, string(matchOrderJSON)); err != nil {
					log.Printf("移除匹配订单失败: %v", err)
					return err
				}

				matchOrder.Amount = matchOrder.Amount.Sub(matchAmount)

				// 如果匹配订单有剩余量，重新添加
				if matchOrder.Amount.GreaterThan(decimal.Zero) {
					if err := rc.AddOrderToBook(matchOrder, pair); err != nil {
						log.Printf("重新添加匹配订单失败: %v", err)
						return err
					}
					if err := tx.Table("orders").Where("order_id = ?", matchOrder.OrderID).Update("status", "PARTIALLY_FILLED").Error; err != nil {
						log.Printf("更新匹配订单状态失败: %v", err)
						return err
					}
				} else {
					if err := tx.Table("orders").Where("order_id = ?", matchOrder.OrderID).Update("status", "FILLED").Error; err != nil {
						log.Printf("更新匹配订单状态失败: %v", err)
						return err
					}
				}
			}
		}

		// 更新新订单状态
		if remainingAmount.LessThanOrEqual(decimal.Zero) {
			if err := tx.Table("orders").Where("order_id = ?", newOrder.OrderID).Update("status", "FILLED").Error; err != nil {
				log.Printf("更新新订单状态失败: %v", err)
				return err
			}
		} else if remainingAmount.LessThan(originalOrder.Amount) {
			if err := tx.Table("orders").Where("order_id = ?", newOrder.OrderID).Update("status", "PARTIALLY_FILLED").Error; err != nil {
				log.Printf("更新新订单状态失败: %v", err)
				return err
			}
		} else {
			if err := tx.Table("orders").Where("order_id = ?", newOrder.OrderID).Update("status", "CLOSE").Error; err != nil {
				log.Printf("更新新订单状态失败: %v", err)
				return err
			}
		}

		// 市价订单不添加到订单簿，直接取消剩余部分
		if remainingAmount.GreaterThan(decimal.Zero) {
			log.Printf("市价订单剩余量 %v 未撮合，取消剩余部分", remainingAmount)
		}

		return nil
	})

	if err != nil {
		return err
	}

	return nil
}

// matchOrdersPriceLimit 撮合限价订单
func matchOrdersPriceLimit(rc *RedisClient, pc *PostgresClient, pair string, newOrder Order) error {
	oppositeKey := "asks:" + pair
	orderKey := "bids:" + pair
	if newOrder.OrderType == "ASK" {
		oppositeKey = "bids:" + pair
		orderKey = "asks:" + pair
	}

	remainingAmount := newOrder.Amount
	originalOrder := newOrder // 保存原始订单用于状态更新

	// 使用 GORM 事务确保数据库一致性
	err := pc.db.Transaction(func(tx *gorm.DB) error {
		for remainingAmount.GreaterThan(decimal.Zero) {
			// 获取对手盘的最佳订单
			bestOrder, bestPrice, err := rc.GetBestOrder(oppositeKey)
			if err != nil {
				log.Printf("获取最佳订单失败: %v", err)
				return err
			}
			if bestOrder == nil {
				log.Printf("无可撮合订单")
				break // 无可撮合订单
			}

			// 检查价格是否匹配
			if (newOrder.OrderType == "BID" && bestPrice.GreaterThan(newOrder.Price)) ||
				(newOrder.OrderType == "ASK" && bestPrice.LessThan(newOrder.Price)) {
				break // 价格不匹配，退出
			}

			// 获取同价格的所有订单，确保 FIFO
			orders, err := rc.GetOrdersByPrice(oppositeKey, bestPrice)
			if err != nil {
				log.Printf("获取同价订单失败: %v", err)
				return err
			}

			if len(orders) == 0 {
				break // 无可用订单
			}

			// 按时间戳升序排序（FIFO）
			sort.Slice(orders, func(i, j int) bool {
				return orders[i].Timestamp < orders[j].Timestamp
			})

			// 撮合订单
			for _, matchOrder := range orders {
				if remainingAmount.LessThanOrEqual(decimal.Zero) {
					break
				}

				// 计算撮合金额
				matchAmount := min(remainingAmount, matchOrder.Amount)
				tradePrice := matchOrder.Price

				// 创建交易记录
				trade := Trade{
					TradeID:    uuid.New().String(),
					BidOrderID: newOrder.OrderID,
					AskOrderID: matchOrder.OrderID,
					Price:      tradePrice,
					Amount:     matchAmount,
				}
				if newOrder.OrderType == "ASK" {
					trade.BidOrderID, trade.AskOrderID = matchOrder.OrderID, newOrder.OrderID
				}

				// 保存交易
				if err := pc.SaveTrade(trade); err != nil {
					log.Printf("保存交易失败: %v", err)
					return err
				}
				if err := rc.PublishTrade(trade); err != nil {
					log.Printf("发布交易失败: %v", err)
					return err
				}

				// 更新订单
				remainingAmount = remainingAmount.Sub(matchAmount)

				// 从 Redis 移除匹配订单
				matchOrderJSON, err := json.Marshal(matchOrder)
				if err != nil {
					log.Printf("序列化匹配订单失败: %v", err)
					return err
				}
				if err := rc.RemoveOrder(oppositeKey, string(matchOrderJSON)); err != nil {
					log.Printf("移除匹配订单失败: %v", err)
					return err
				}

				matchOrder.Amount = matchOrder.Amount.Sub(matchAmount)

				// 如果匹配订单有剩余量，重新添加
				if matchOrder.Amount.GreaterThan(decimal.Zero) {
					if err := rc.AddOrderToBook(matchOrder, pair); err != nil {
						log.Printf("重新添加匹配订单失败: %v", err)
						return err
					}
					// 更新匹配订单状态为 PARTIALLY_FILLED
					if err := tx.Table("orders").Where("order_id = ?", matchOrder.OrderID).Update("status", "PARTIALLY_FILLED").Error; err != nil {
						log.Printf("更新匹配订单状态失败: %v", err)
						return err
					}
				} else {
					// 更新匹配订单状态为 FILLED
					if err := tx.Table("orders").Where("order_id = ?", matchOrder.OrderID).Update("status", "FILLED").Error; err != nil {
						log.Printf("更新匹配订单状态失败: %v", err)
						return err
					}
				}
			}
		}

		// 更新新订单状态
		if remainingAmount.LessThanOrEqual(decimal.Zero) {
			// 新订单完全撮合
			if err := tx.Table("orders").Where("order_id = ?", newOrder.OrderID).Update("status", "FILLED").Error; err != nil {
				log.Printf("更新新订单状态失败: %v", err)
				return err
			}
		} else {
			// 从 Redis 移除当前订单状态
			newOrderJSON, err := json.Marshal(newOrder)
			if err != nil {
				log.Printf("序列化新订单失败: %v", err)
				return err
			}

			if err := rc.RemoveOrder(orderKey, string(newOrderJSON)); err != nil {
				log.Printf("移除新订单失败: %v", err)
				return err
			}

			// 更新新订单量
			newOrder.Amount = remainingAmount
			if remainingAmount.LessThan(originalOrder.Amount) {
				// 部分撮合
				if err := tx.Table("orders").Where("order_id = ?", newOrder.OrderID).Update("status", "PARTIALLY_FILLED").Error; err != nil {
					log.Printf("更新新订单状态失败: %v", err)
					return err
				}
			}

			// 剩余订单重新添加到订单簿
			if err := rc.AddOrderToBook(newOrder, pair); err != nil {
				log.Printf("添加剩余订单失败: %v", err)
				return err
			}
		}

		return nil
	})

	if err != nil {
		return err
	}

	return nil
}

func min(a, b decimal.Decimal) decimal.Decimal {
	if a.LessThan(b) {
		return a
	}
	return b
}
