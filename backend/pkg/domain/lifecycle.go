package domain

import (
	"time"
)

// LifecycleStages 域名生命周期各阶段时间
type LifecycleStages struct {
	ExpiryDate         *time.Time `json:"expiryDate"`         // 到期日期
	GracePeriodEnd     *time.Time `json:"gracePeriodEnd"`     // 宽限期结束（到期后30天）
	RedemptionStart    *time.Time `json:"redemptionStart"`    // 赎回期开始（宽限期结束后）
	RedemptionEnd      *time.Time `json:"redemptionEnd"`      // 赎回期结束（赎回期开始后30天）
	PendingDeleteStart *time.Time `json:"pendingDeleteStart"` // 待删除期开始（赎回期结束后）
	PendingDeleteEnd   *time.Time `json:"pendingDeleteEnd"`   // 待删除期结束（待删除期开始后5天）
	AvailableDate      *time.Time `json:"availableDate"`      // 可注册时间（待删除期结束后）
}

// CalculateLifecycleStages 根据到期日期计算域名生命周期各阶段时间
// 标准流程：到期 -> 宽限期(30天) -> 赎回期(30天) -> 待删除期(5天) -> 可注册
func CalculateLifecycleStages(expiryDate *time.Time) *LifecycleStages {
	if expiryDate == nil {
		return nil
	}

	stages := &LifecycleStages{
		ExpiryDate: expiryDate,
	}

	// 宽限期：到期后30天
	gracePeriodEnd := expiryDate.AddDate(0, 0, 30)
	stages.GracePeriodEnd = &gracePeriodEnd

	// 赎回期：宽限期结束后30天
	redemptionStart := gracePeriodEnd
	stages.RedemptionStart = &redemptionStart
	redemptionEnd := redemptionStart.AddDate(0, 0, 30)
	stages.RedemptionEnd = &redemptionEnd

	// 待删除期：赎回期结束后5天
	pendingDeleteStart := redemptionEnd
	stages.PendingDeleteStart = &pendingDeleteStart
	pendingDeleteEnd := pendingDeleteStart.AddDate(0, 0, 5)
	stages.PendingDeleteEnd = &pendingDeleteEnd

	// 可注册时间：待删除期结束后
	availableDate := pendingDeleteEnd
	stages.AvailableDate = &availableDate

	return stages
}

// GetCurrentStage 根据当前时间和到期日期，判断域名当前处于哪个阶段
func GetCurrentStage(expiryDate *time.Time, currentStatus string) string {
	if expiryDate == nil {
		return "unknown"
	}

	now := time.Now()
	stages := CalculateLifecycleStages(expiryDate)

	// 如果已经有明确的状态，优先使用
	if currentStatus != "" && currentStatus != "unknown" && currentStatus != "registered" {
		return currentStatus
	}

	// 根据时间判断当前阶段
	if now.Before(*expiryDate) {
		return "registered"
	}

	if stages.GracePeriodEnd != nil && now.Before(*stages.GracePeriodEnd) {
		return "grace_period"
	}

	if stages.RedemptionEnd != nil && now.Before(*stages.RedemptionEnd) {
		return "redemption"
	}

	if stages.PendingDeleteEnd != nil && now.Before(*stages.PendingDeleteEnd) {
		return "pending_delete"
	}

	if stages.AvailableDate != nil && now.After(*stages.AvailableDate) {
		return "available"
	}

	return "expired"
}
