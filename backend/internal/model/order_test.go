package model

import "testing"

func TestCanTransitionLegal(t *testing.T) {
	legal := [][2]string{
		{OrderStatusPending, OrderStatusPaid},
		{OrderStatusPending, OrderStatusCancelled},
		{OrderStatusPaid, OrderStatusActive},
		{OrderStatusPaid, OrderStatusProvisionFailed},
		{OrderStatusPaid, OrderStatusRefunded},
		{OrderStatusProvisionFailed, OrderStatusPaid},
		{OrderStatusProvisionFailed, OrderStatusActive},
		{OrderStatusProvisionFailed, OrderStatusRefunded},
	}
	for _, e := range legal {
		if !CanTransition(e[0], e[1]) {
			t.Errorf("合法迁移 %s → %s 被拒绝", e[0], e[1])
		}
	}
}

func TestCanTransitionIllegal(t *testing.T) {
	illegal := [][2]string{
		// 终态无出边
		{OrderStatusActive, OrderStatusPending},
		{OrderStatusActive, OrderStatusRefunded},
		{OrderStatusActive, OrderStatusCancelled},
		{OrderStatusCancelled, OrderStatusPaid},
		{OrderStatusCancelled, OrderStatusPending},
		{OrderStatusRefunded, OrderStatusPaid},
		{OrderStatusRefunded, OrderStatusActive},
		// 跳级/回退
		{OrderStatusPending, OrderStatusActive},   // 未支付不能直接开通
		{OrderStatusPending, OrderStatusRefunded}, // 未支付无款可退
		{OrderStatusPending, OrderStatusProvisionFailed},
		{OrderStatusPaid, OrderStatusPending},   // 已支付不可回退待支付
		{OrderStatusPaid, OrderStatusCancelled}, // 已支付不可取消（退款留批 4/后台）
		{OrderStatusProvisionFailed, OrderStatusPending},
		{OrderStatusProvisionFailed, OrderStatusCancelled},
		// 自迁移与未知状态
		{OrderStatusPending, OrderStatusPending},
		{"", OrderStatusPaid},
		{OrderStatusPaid, ""},
		{"Unknown", OrderStatusActive},
		{OrderStatusPaid, "Unknown"},
	}
	for _, e := range illegal {
		if CanTransition(e[0], e[1]) {
			t.Errorf("非法迁移 %s → %s 未被拒绝", e[0], e[1])
		}
	}
}
