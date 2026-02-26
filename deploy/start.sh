#!/bin/bash

# 域名抢注平台启动脚本

cd "$(dirname "$0")/../backend/api"

# 检查是否已经在运行
if pgrep -f "go run domain.go" > /dev/null; then
    echo "⚠️  后端服务已在运行中"
    exit 1
fi

# 启动后端服务
nohup go run domain.go -f etc/domain.yaml > /tmp/domain-snatch.log 2>&1 &
echo "✓ 后端服务已启动，PID: $!"

# 等待启动
sleep 3

# 检查服务状态
if pgrep -f "go run domain.go" > /dev/null; then
    echo "✓ 后端服务运行正常"
    echo "✓ API: http://localhost:8888"
    echo "✓ 日志: tail -f /tmp/domain-snatch.log"
else
    echo "✗ 后端服务启动失败"
    exit 1
fi
