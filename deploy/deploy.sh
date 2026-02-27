#!/bin/bash
# 阿里云 ECS 部署脚本，由云效流水线在主机上执行
# 用法: deploy.sh restart
# 解压后运行目录为应用根目录，包含 domain-snatch 二进制和 etc/ 配置

set -e
APP_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$APP_DIR"

case "${1:-restart}" in
  restart)
    # 优先使用 systemd
    if command -v systemctl >/dev/null 2>&1 && systemctl list-unit-files --type=service | grep -q domain-snatch; then
      systemctl restart domain-snatch || true
      exit 0
    fi
    # 否则用进程方式：停掉旧进程再启动
    if [ -x ./domain-snatch ]; then
      pkill -f "domain-snatch.*etc/domain" || true
      sleep 2
      nohup ./domain-snatch -f etc/domain.yaml >> /var/log/domain-snatch.log 2>&1 &
      echo "domain-snatch started (PID: $!)"
    else
      echo "error: ./domain-snatch not found in $APP_DIR"
      exit 1
    fi
    ;;
  stop)
    pkill -f "domain-snatch.*etc/domain" || true
    ;;
  *)
    echo "Usage: $0 restart|stop"
    exit 1
    ;;
esac
