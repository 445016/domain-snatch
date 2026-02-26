#!/bin/bash

# 域名抢注平台停止脚本

echo "正在停止后端服务..."

# 查找并停止进程
pkill -f "go run domain.go"

sleep 2

# 检查是否已停止
if pgrep -f "go run domain.go" > /dev/null; then
    echo "⚠️  无法正常停止，强制结束..."
    pkill -9 -f "go run domain.go"
fi

echo "✓ 后端服务已停止"
