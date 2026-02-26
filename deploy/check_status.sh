#!/bin/bash

echo "=== 域名抢注平台 - 服务状态检查 ==="
echo ""

# 检查后端服务
if pgrep -f "go run domain.go" > /dev/null; then
    PID=$(pgrep -f "go run domain.go" | head -1)
    echo "✓ 后端服务运行中 (PID: $PID)"
    echo "  API: http://localhost:8888"
else
    echo "✗ 后端服务未运行"
    echo "  启动命令: cd /Users/dyxc/Desktop/xj/deploy && ./start.sh"
fi

# 检查前端服务
if lsof -i :5174 > /dev/null 2>&1; then
    echo "✓ 前端服务运行中"
    echo "  地址: http://127.0.0.1:5174"
else
    echo "⚠️  前端服务未运行"
    echo "  启动命令: cd /Users/dyxc/Desktop/xj/frontend && npm run dev"
fi

# 检查数据库连接
echo ""
echo "=== 数据库状态 ==="
mysql -h 127.0.0.1 -u root -p123456 -D domain_snatch -e "
SELECT 
    (SELECT COUNT(*) FROM domains) as '总域名数',
    (SELECT COUNT(*) FROM domains WHERE monitor=1) as '监控中',
    (SELECT COUNT(*) FROM domains WHERE last_checked IS NOT NULL) as '已检查',
    (SELECT MAX(last_checked) FROM domains) as '最后检查时间'
" 2>/dev/null || echo "⚠️  数据库连接失败"

echo ""
echo "=== 定时任务配置 ==="
echo "✓ WHOIS巡检: 每天凌晨 2:00"
echo "✓ 到期提醒: 每天凌晨 2:00"  
echo "✓ 抢注检查: 每小时一次"

echo ""
TOMORROW=$(date -v+1d "+%Y-%m-%d")
echo "📅 下次自动检查时间: $TOMORROW 02:00:00"
