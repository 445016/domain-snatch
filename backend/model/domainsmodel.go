package model

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

var _ DomainsModel = (*customDomainsModel)(nil)

type (
	DomainsModel interface {
		domainsModel
		withSession(session sqlx.Session) DomainsModel
		FindList(ctx context.Context, page, pageSize int64, status, keyword string, monitor int64, expiryStart, expiryEnd, sortField, sortOrder string) ([]*Domains, error)
		Count(ctx context.Context, status, keyword string, monitor int64, expiryStart, expiryEnd string) (int64, error)
		CountByStatus(ctx context.Context, status string) (int64, error)
		CountMonitor(ctx context.Context) (int64, error)
		CountExpiringSoon(ctx context.Context, days int64) (int64, error)
		FindMonitored(ctx context.Context) ([]*Domains, error)
		FindExpiringSoon(ctx context.Context, days int64) ([]*Domains, error)
		FindAvailableOrPastDomains(ctx context.Context) ([]*Domains, error)
		FindDomainsAtKeyTimePoints(ctx context.Context) ([]*Domains, error)
		FindDomainsNearKeyTimePoints(ctx context.Context, minutesBefore, minutesAfter int) ([]*Domains, error)
		FindDomainsNearAvailable(ctx context.Context, hoursBefore int) ([]*Domains, error)
		FindDomainsWithinMinutesToAvailable(ctx context.Context, minutes int) ([]*Domains, error)
		FindDomainsWithExpiredOrNullExpiry(ctx context.Context) ([]*Domains, error)
		FindDomainsWithExpiredOrNullExpiryAsOf(ctx context.Context, asOf time.Time) ([]*Domains, error)
		FindPendingDeleteDomainsNearAvailable(ctx context.Context, hoursBefore int) ([]*Domains, error)
	}

	customDomainsModel struct {
		*defaultDomainsModel
	}
)

func NewDomainsModel(conn sqlx.SqlConn) DomainsModel {
	return &customDomainsModel{
		defaultDomainsModel: newDomainsModel(conn),
	}
}

func (m *customDomainsModel) withSession(session sqlx.Session) DomainsModel {
	return NewDomainsModel(sqlx.NewSqlConnFromSession(session))
}

func (m *customDomainsModel) FindList(ctx context.Context, page, pageSize int64, status, keyword string, monitor int64, expiryStart, expiryEnd, sortField, sortOrder string) ([]*Domains, error) {
	where := "1=1"
	args := make([]interface{}, 0)
	if status != "" {
		where += " AND `status` = ?"
		args = append(args, status)
	}
	if keyword != "" {
		where += " AND `domain` LIKE ?"
		args = append(args, "%"+keyword+"%")
	}
	if monitor >= 0 {
		where += " AND `monitor` = ?"
		args = append(args, monitor)
	}
	if expiryStart != "" {
		where += " AND `expiry_date` >= ?"
		args = append(args, expiryStart+" 00:00:00")
	}
	if expiryEnd != "" {
		where += " AND `expiry_date` <= ?"
		args = append(args, expiryEnd+" 23:59:59")
	}

	// 排序字段验证和清理
	sortField = strings.TrimSpace(sortField)
	sortOrder = strings.TrimSpace(sortOrder)

	allowedSortFields := map[string]bool{
		"id": true, "domain": true, "status": true,
		"expiry_date": true, "creation_date": true, "last_checked": true,
	}
	if !allowedSortFields[sortField] {
		sortField = "expiry_date" // 默认按到期时间排序
	}
	if sortOrder != "asc" && sortOrder != "desc" {
		sortOrder = "asc" // 默认升序：由近到远
	}

	// 默认逻辑：先按状态排序，然后按到期时间排序
	// 如果用户选择了到期时间排序，则先按到期时间排序，然后按状态排序
	var orderBy string
	if sortField == "expiry_date" {
		// 按到期时间排序：先按到期时间，然后按状态优先级排序
		statusOrder := "FIELD(`status`, 'unknown', 'expired', 'restricted', 'registered', 'grace_period', 'redemption', 'pending_delete', 'available') DESC"
		if sortOrder == "desc" {
			orderBy = fmt.Sprintf("`expiry_date` IS NULL, `expiry_date` DESC, %s", statusOrder)
		} else {
			orderBy = fmt.Sprintf("`expiry_date` IS NULL, `expiry_date` ASC, %s", statusOrder)
		}
	} else if sortField == "status" || sortField == "" {
		// 默认排序：先按状态优先级排序，然后按到期时间排序
		// 状态优先级：available > pending_delete > ... > registered > restricted > expired > unknown
		// FIELD 反转列表：unknown(1) < expired(2) < restricted(3) < registered(4) < ... < available(8)
		if sortOrder == "desc" {
			orderBy = "FIELD(`status`, 'unknown', 'expired', 'restricted', 'registered', 'grace_period', 'redemption', 'pending_delete', 'available') DESC, `expiry_date` IS NULL, `expiry_date` DESC"
		} else {
			orderBy = "FIELD(`status`, 'unknown', 'expired', 'restricted', 'registered', 'grace_period', 'redemption', 'pending_delete', 'available') ASC, `expiry_date` IS NULL, `expiry_date` ASC"
		}
	} else {
		// 用户选择了其他排序字段，按用户选择的字段排序
		statusOrder := "FIELD(`status`, 'unknown', 'expired', 'restricted', 'registered', 'grace_period', 'redemption', 'pending_delete', 'available') DESC"
		if sortOrder == "desc" {
			orderBy = fmt.Sprintf("`%s` DESC, %s, `expiry_date` IS NULL, `expiry_date` DESC", sortField, statusOrder)
		} else {
			orderBy = fmt.Sprintf("`%s` ASC, %s, `expiry_date` IS NULL, `expiry_date` ASC", sortField, statusOrder)
		}
	}

	offset := (page - 1) * pageSize
	args = append(args, offset, pageSize)
	query := fmt.Sprintf("SELECT %s FROM %s WHERE %s ORDER BY %s LIMIT ?, ?", domainsRows, m.table, where, orderBy)

	// 调试日志：打印排序参数和 SQL
	log.Printf("[FindList] sortField=%s, sortOrder=%s, orderBy=%s", sortField, sortOrder, orderBy)
	log.Printf("[FindList] SQL: %s", query)

	var resp []*Domains
	err := m.conn.QueryRowsCtx(ctx, &resp, query, args...)
	return resp, err
}

func (m *customDomainsModel) Count(ctx context.Context, status, keyword string, monitor int64, expiryStart, expiryEnd string) (int64, error) {
	where := "1=1"
	args := make([]interface{}, 0)
	if status != "" {
		where += " AND `status` = ?"
		args = append(args, status)
	}
	if keyword != "" {
		where += " AND `domain` LIKE ?"
		args = append(args, "%"+keyword+"%")
	}
	if monitor >= 0 {
		where += " AND `monitor` = ?"
		args = append(args, monitor)
	}
	if expiryStart != "" {
		where += " AND `expiry_date` >= ?"
		args = append(args, expiryStart)
	}
	if expiryEnd != "" {
		where += " AND `expiry_date` <= ?"
		args = append(args, expiryEnd)
	}
	query := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE %s", m.table, where)
	var count int64
	err := m.conn.QueryRowCtx(ctx, &count, query, args...)
	return count, err
}

func (m *customDomainsModel) CountByStatus(ctx context.Context, status string) (int64, error) {
	query := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE `status` = ?", m.table)
	var count int64
	err := m.conn.QueryRowCtx(ctx, &count, query, status)
	return count, err
}

func (m *customDomainsModel) CountMonitor(ctx context.Context) (int64, error) {
	query := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE `monitor` = 1", m.table)
	var count int64
	err := m.conn.QueryRowCtx(ctx, &count, query)
	return count, err
}

func (m *customDomainsModel) CountExpiringSoon(ctx context.Context, days int64) (int64, error) {
	query := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE `status` = 'registered' AND `expiry_date` IS NOT NULL AND `expiry_date` <= DATE_ADD(NOW(), INTERVAL ? DAY)", m.table)
	var count int64
	err := m.conn.QueryRowCtx(ctx, &count, query, days)
	return count, err
}

func (m *customDomainsModel) FindMonitored(ctx context.Context) ([]*Domains, error) {
	query := fmt.Sprintf("SELECT %s FROM %s WHERE `monitor` = 1", domainsRows, m.table)
	var resp []*Domains
	err := m.conn.QueryRowsCtx(ctx, &resp, query)
	return resp, err
}

func (m *customDomainsModel) FindExpiringSoon(ctx context.Context, days int64) ([]*Domains, error) {
	query := fmt.Sprintf("SELECT %s FROM %s WHERE `status` = 'registered' AND `expiry_date` IS NOT NULL AND `expiry_date` <= DATE_ADD(NOW(), INTERVAL ? DAY) AND `monitor` = 1 ORDER BY `expiry_date` ASC", domainsRows, m.table)
	var resp []*Domains
	err := m.conn.QueryRowsCtx(ctx, &resp, query, days)
	return resp, err
}

// FindAvailableOrPastDomains 查询可注册时间 <= 当天的所有监控域名
// 可注册时间 = 到期日期 + 65天（宽限期30天 + 赎回期30天 + 待删除期5天）
func (m *customDomainsModel) FindAvailableOrPastDomains(ctx context.Context) ([]*Domains, error) {
	query := fmt.Sprintf("SELECT %s FROM %s WHERE `expiry_date` IS NOT NULL AND DATE_ADD(`expiry_date`, INTERVAL 65 DAY) <= CURDATE() AND `monitor` = 1 ORDER BY `expiry_date` ASC", domainsRows, m.table)
	var resp []*Domains
	err := m.conn.QueryRowsCtx(ctx, &resp, query)
	return resp, err
}

// FindDomainsAtKeyTimePoints 查询所有在关键时间点的域名（今天可能发生状态变化的域名）
// 包括：到期日当天、宽限期结束当天、赎回期结束当天、待删除期结束当天
func (m *customDomainsModel) FindDomainsAtKeyTimePoints(ctx context.Context) ([]*Domains, error) {
	query := fmt.Sprintf(`SELECT %s FROM %s 
		WHERE expiry_date IS NOT NULL 
		  AND monitor = 1
		  AND (
		    DATE(expiry_date) = CURDATE()  -- 到期日当天
		    OR DATE(DATE_ADD(expiry_date, INTERVAL 30 DAY)) = CURDATE()  -- 宽限期结束当天
		    OR DATE(DATE_ADD(expiry_date, INTERVAL 60 DAY)) = CURDATE()  -- 赎回期结束当天
		    OR DATE(DATE_ADD(expiry_date, INTERVAL 65 DAY)) = CURDATE()  -- 待删除期结束当天
		  )
		ORDER BY expiry_date ASC`, domainsRows, m.table)
	var resp []*Domains
	err := m.conn.QueryRowsCtx(ctx, &resp, query)
	return resp, err
}

// FindDomainsNearKeyTimePoints 查询即将到达关键时间点的域名（基于时分秒，在指定时间窗口内）
// minutesBefore: 提前多少分钟开始检测（例如：5表示提前5分钟开始检测）
// minutesAfter: 延后多少分钟结束检测（例如：5表示延后5分钟结束检测）
func (m *customDomainsModel) FindDomainsNearKeyTimePoints(ctx context.Context, minutesBefore, minutesAfter int) ([]*Domains, error) {
	now := time.Now()
	startTime := now.Add(-time.Duration(minutesBefore) * time.Minute)
	endTime := now.Add(time.Duration(minutesAfter) * time.Minute)

	query := fmt.Sprintf(`SELECT %s FROM %s 
		WHERE expiry_date IS NOT NULL 
		  AND monitor = 1
		  AND (
		    -- 到期日时间点
		    (expiry_date >= ? AND expiry_date <= ?)
		    -- 宽限期结束时间点（到期日+30天）
		    OR (DATE_ADD(expiry_date, INTERVAL 30 DAY) >= ? AND DATE_ADD(expiry_date, INTERVAL 30 DAY) <= ?)
		    -- 赎回期结束时间点（到期日+60天）
		    OR (DATE_ADD(expiry_date, INTERVAL 60 DAY) >= ? AND DATE_ADD(expiry_date, INTERVAL 60 DAY) <= ?)
		    -- 待删除期结束时间点（到期日+65天）
		    OR (DATE_ADD(expiry_date, INTERVAL 65 DAY) >= ? AND DATE_ADD(expiry_date, INTERVAL 65 DAY) <= ?)
		  )
		ORDER BY expiry_date ASC`, domainsRows, m.table)

	var resp []*Domains
	err := m.conn.QueryRowsCtx(ctx, &resp, query,
		startTime, endTime, // 到期日
		startTime, endTime, // 宽限期结束
		startTime, endTime, // 赎回期结束
		startTime, endTime, // 待删除期结束
	)
	return resp, err
}

// FindDomainsNearAvailable 查询即将变成可注册状态的域名
// hoursBefore: 提前多少小时开始实时检测（例如：24表示提前24小时开始实时检测）
// 查询条件：状态为 pending_delete，且可注册时间在未来 hoursBefore 小时内
func (m *customDomainsModel) FindDomainsNearAvailable(ctx context.Context, hoursBefore int) ([]*Domains, error) {
	now := time.Now()
	thresholdTime := now.Add(time.Duration(hoursBefore) * time.Hour)

	query := fmt.Sprintf(`SELECT %s FROM %s 
		WHERE expiry_date IS NOT NULL 
		  AND monitor = 1
		  AND status = 'pending_delete'
		  AND DATE_ADD(expiry_date, INTERVAL 65 DAY) >= ?
		  AND DATE_ADD(expiry_date, INTERVAL 65 DAY) <= ?
		ORDER BY expiry_date ASC`, domainsRows, m.table)

	var resp []*Domains
	err := m.conn.QueryRowsCtx(ctx, &resp, query, now, thresholdTime)
	return resp, err
}

// FindDomainsWithinMinutesToAvailable 查询可注册时间在指定分钟数内的域名
// minutes: 距离可注册时间还有多少分钟（例如：5表示5分钟内可注册）
func (m *customDomainsModel) FindDomainsWithinMinutesToAvailable(ctx context.Context, minutes int) ([]*Domains, error) {
	now := time.Now()
	thresholdTime := now.Add(time.Duration(minutes) * time.Minute)

	query := fmt.Sprintf(`SELECT %s FROM %s 
		WHERE expiry_date IS NOT NULL 
		  AND monitor = 1
		  AND status = 'pending_delete'
		  AND DATE_ADD(expiry_date, INTERVAL 65 DAY) >= ?
		  AND DATE_ADD(expiry_date, INTERVAL 65 DAY) <= ?
		ORDER BY expiry_date ASC`, domainsRows, m.table)

	var resp []*Domains
	err := m.conn.QueryRowsCtx(ctx, &resp, query, now, thresholdTime)
	return resp, err
}

// FindDomainsWithExpiredOrNullExpiry 查询「当前状态的结束时间 <= 当前时间」的域名，用于状态更新
// 各状态对应的结束时间：registered→到期日；expired/grace_period→+30天；redemption→+60天；pending_delete/available→+65天；unknown→expiry_date IS NULL
// restricted 不参与（无结束时间，通过状态过滤掉）；不限制 monitor
func (m *customDomainsModel) FindDomainsWithExpiredOrNullExpiry(ctx context.Context) ([]*Domains, error) {
	query := fmt.Sprintf(`SELECT %s FROM %s 
		WHERE (status = 'registered' AND (expiry_date IS NULL OR expiry_date <= NOW()))
		   OR (status = 'expired' AND expiry_date IS NOT NULL AND DATE_ADD(expiry_date, INTERVAL 30 DAY) <= NOW())
		   OR (status = 'grace_period' AND expiry_date IS NOT NULL AND DATE_ADD(expiry_date, INTERVAL 30 DAY) <= NOW())
		   OR (status = 'redemption' AND expiry_date IS NOT NULL AND DATE_ADD(expiry_date, INTERVAL 60 DAY) <= NOW())
		   OR (status = 'pending_delete' AND expiry_date IS NOT NULL AND DATE_ADD(expiry_date, INTERVAL 65 DAY) <= NOW())
		   OR (status = 'available' AND expiry_date IS NOT NULL AND DATE_ADD(expiry_date, INTERVAL 65 DAY) <= NOW())
		   OR (status = 'unknown' AND expiry_date IS NULL)
		ORDER BY expiry_date ASC, id ASC`, domainsRows, m.table)

	var resp []*Domains
	err := m.conn.QueryRowsCtx(ctx, &resp, query)
	return resp, err
}

// FindDomainsWithExpiredOrNullExpiryAsOf 与 FindDomainsWithExpiredOrNullExpiry 逻辑相同，但用 asOf 作为参考时间替代 NOW()
func (m *customDomainsModel) FindDomainsWithExpiredOrNullExpiryAsOf(ctx context.Context, asOf time.Time) ([]*Domains, error) {
	query := fmt.Sprintf(`SELECT %s FROM %s 
		WHERE (status = 'registered' AND (expiry_date IS NULL OR expiry_date <= ?))
		   OR (status = 'expired' AND expiry_date IS NOT NULL AND DATE_ADD(expiry_date, INTERVAL 30 DAY) <= ?)
		   OR (status = 'grace_period' AND expiry_date IS NOT NULL AND DATE_ADD(expiry_date, INTERVAL 30 DAY) <= ?)
		   OR (status = 'redemption' AND expiry_date IS NOT NULL AND DATE_ADD(expiry_date, INTERVAL 60 DAY) <= ?)
		   OR (status = 'pending_delete' AND expiry_date IS NOT NULL AND DATE_ADD(expiry_date, INTERVAL 65 DAY) <= ?)
		   OR (status = 'available' AND expiry_date IS NOT NULL AND DATE_ADD(expiry_date, INTERVAL 65 DAY) <= ?)
		   OR (status = 'unknown' AND expiry_date IS NULL)
		ORDER BY expiry_date ASC, id ASC`, domainsRows, m.table)
	ref := asOf.Truncate(time.Second)
	var resp []*Domains
	err := m.conn.QueryRowsCtx(ctx, &resp, query, ref, ref, ref, ref, ref, ref)
	return resp, err
}

// FindDomainByExpiryDate 根据到期日期查询域名（用于诊断）
// 查询指定到期日期附近的域名，不限制 monitor 状态
func (m *customDomainsModel) FindDomainByExpiryDate(ctx context.Context, expiryDate time.Time, daysBefore, daysAfter int) ([]*Domains, error) {
	startDate := expiryDate.AddDate(0, 0, -daysBefore)
	endDate := expiryDate.AddDate(0, 0, daysAfter)

	query := fmt.Sprintf(`SELECT %s FROM %s 
		WHERE expiry_date IS NOT NULL
		  AND expiry_date >= ?
		  AND expiry_date <= ?
		ORDER BY expiry_date ASC, id ASC`, domainsRows, m.table)

	var resp []*Domains
	err := m.conn.QueryRowsCtx(ctx, &resp, query, startDate, endDate)
	return resp, err
}

// FindPendingDeleteDomainsNearAvailable 查询处于删除期且接近可注册时间的域名
// hoursBefore: 提前多少小时开始检测（例如：24表示未来24小时内可注册的域名）
func (m *customDomainsModel) FindPendingDeleteDomainsNearAvailable(ctx context.Context, hoursBefore int) ([]*Domains, error) {
	query := fmt.Sprintf(`SELECT %s FROM %s 
		WHERE status = 'pending_delete'
		  AND monitor = 1
		  AND expiry_date IS NOT NULL
		  AND DATE_ADD(expiry_date, INTERVAL 65 DAY) >= NOW()
		  AND DATE_ADD(expiry_date, INTERVAL 65 DAY) <= DATE_ADD(NOW(), INTERVAL ? HOUR)
		ORDER BY expiry_date ASC`, domainsRows, m.table)

	var resp []*Domains
	err := m.conn.QueryRowsCtx(ctx, &resp, query, hoursBefore)
	return resp, err
}
