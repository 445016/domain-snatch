package snatch

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/zeromicro/go-zero/core/stores/redis"
)

const delayQueueKey = "snatch:delay_queue"

// DelayQueue 抢注延迟队列：score = 执行时间戳，member = taskID
type DelayQueue struct {
	rds *redis.Redis
}

// NewDelayQueue 使用 go-zero Redis 创建延迟队列
func NewDelayQueue(rds *redis.Redis) *DelayQueue {
	return &DelayQueue{rds: rds}
}

// Add 将任务加入队列，在 executeAt 时刻被消费
func (q *DelayQueue) Add(ctx context.Context, taskID int64, executeAt time.Time) error {
	member := fmt.Sprintf("%d", taskID)
	score := executeAt.Unix()
	_, err := q.rds.ZaddCtx(ctx, delayQueueKey, score, member)
	return err
}

// Poll 取出一个已到点的任务（score <= now），并从队列移除；若无则 ok=false
func (q *DelayQueue) Poll(ctx context.Context) (taskID int64, ok bool, err error) {
	now := time.Now().Unix()
	pairs, err := q.rds.ZrangebyscoreWithScoresAndLimitCtx(ctx, delayQueueKey, 0, now, 0, 1)
	if err != nil || len(pairs) == 0 {
		return 0, false, err
	}
	member := pairs[0].Key
	_, _ = q.rds.ZremCtx(ctx, delayQueueKey, member)
	id, err := strconv.ParseInt(member, 10, 64)
	if err != nil {
		return 0, false, err
	}
	return id, true, nil
}
