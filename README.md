# 域名抢注监控平台

基于 Go-Zero + React + Ant Design 构建的域名抢注监控平台，支持批量域名监控、WHOIS 查询、到期提醒、抢注管理，并通过飞书 Webhook 发送通知。

## 技术栈

- **后端**: Go-Zero (API 服务 + 定时任务)
- **前端**: React + TypeScript + Ant Design + Vite
- **数据库**: MySQL 8.0
- **缓存**: Redis
- **通知**: 飞书 Webhook 机器人

## 功能特性

- 域名管理：添加、删除、批量导入（Excel）
- 域名监控：开启/关闭域名监控，定时 WHOIS 巡检
- WHOIS 查询：实时查询域名 WHOIS 信息
- 到期提醒：域名即将到期自动发送飞书通知
- 抢注管理：创建抢注任务、跟踪任务状态
- 仪表盘：域名统计总览
- 用户认证：JWT 登录鉴权

## 快速开始

### 环境要求

- Go 1.22+
- Node.js 16+
- MySQL 8.0
- Redis

### 1. 初始化数据库

```bash
mysql -u root -p < deploy/sql/init.sql
```

默认管理员账号: `admin` / `admin123`

### 2. 启动后端

```bash
cd backend
go mod tidy
cd api
go run domain.go -f etc/domain.yaml
```

后端服务运行在 http://localhost:8888

### 3. 启动前端

```bash
cd frontend
npm install
npm run dev
```

前端服务运行在 http://localhost:5173

### 4. 配置飞书通知

1. 在飞书群中添加自定义机器人
2. 复制 Webhook URL
3. 在平台"通知设置"页面粘贴 URL 并启用

## Docker 部署

```bash
cd deploy
docker-compose up -d
```

- 前端: http://localhost:3000
- 后端 API: http://localhost:8888

## 项目结构

```
├── backend/
│   ├── api/              # Go-Zero API 服务
│   │   ├── domain.api    # API 定义
│   │   ├── domain.go     # 入口文件
│   │   ├── etc/          # 配置
│   │   └── internal/     # handler / logic / svc / types
│   ├── model/            # 数据库模型
│   ├── cron/             # 定时任务
│   └── pkg/              # 公共包
│       ├── whois/        # WHOIS 查询
│       ├── feishu/       # 飞书通知
│       └── excel/        # Excel 解析
├── frontend/
│   └── src/
│       ├── pages/        # 页面组件
│       ├── components/   # 公共组件
│       ├── api/          # API 请求
│       └── store/        # 状态管理
├── deploy/
│   ├── sql/              # 数据库脚本
│   ├── docker-compose.yml
│   └── Dockerfile.*
└── README.md
```

## 配置说明

后端配置文件: `backend/api/etc/domain.yaml`

```yaml
Name: domain
Host: 0.0.0.0
Port: 8888

Auth:
  AccessSecret: "your-secret-key"
  AccessExpire: 86400

Mysql:
  DataSource: "root:password@tcp(127.0.0.1:3306)/domain_snatch?charset=utf8mb4&parseTime=true&loc=Local"

Cache:
  - Host: "127.0.0.1:6379"
```
