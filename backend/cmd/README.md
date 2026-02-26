# 域名管理命令行工具

**数据库配置**：全项目统一走配置文件，不使用 dsn 参数。命令行通过 `-f` 指定配置文件路径（默认 `api/etc/domain.yaml`），其中的 `Mysql.DataSource` 即数据库连接；API 服务通过启动参数 `-f` 加载同一份配置。

**多环境**：当使用默认的 `-f` 时，若设置环境变量 `APP_ENV`，会自动加载同目录下的 `domain-{APP_ENV}.yaml`（如 `APP_ENV=prod` 加载 `domain-prod.yaml`）。也可直接指定 `-f api/etc/domain-prod.yaml`。

## 目录结构

```
cmd/
├── check_domain/     # 检查域名 WHOIS（单个/全部/指定时间）
│   ├── check_domain.go
│   └── README.md
├── import_domains/  # 导入域名（支持 Excel 和文本文件）
│   └── import_domains.go
├── snatch_domain/   # 手动抢注（先发飞书通知再执行）
│   └── snatch_domain.go
├── check.sh         # 快速检查脚本
├── smart_check.sh   # 智能批量检查脚本
└── README.md
```

## 工具说明

### 1. 检查域名 WHOIS (check_domain)

支持三种模式：单个域名、全部域名、按「当前状态结束时间」筛选。数据库连接从项目配置文件读取（默认 `api/etc/domain.yaml`）。

详见 [check_domain/README.md](check_domain/README.md)。

```bash
cd backend

# 检查单个域名
go run ./cmd/check_domain -domain=baidu.com

# 检查所有域名
go run ./cmd/check_domain

# 按时间：只更新「当前状态结束时间≤当前时间」的域名（与 cron 一致）
go run ./cmd/check_domain -time

# 仅检查指定状态的域名
go run ./cmd/check_domain -status=unknown
```

### 2. 导入域名 (import_domains)

从 Excel 或文本文件批量导入域名到数据库，**解析与入库逻辑与 API `/import` 一致**（共用 `pkg/excel` 与 `model.DomainsModel`）。

```bash
cd backend

# 导入 Excel 文件（默认使用 api/etc/domain.yaml 的数据库配置）
go run ./cmd/import_domains -file=/path/to/domains.xlsx

# 导入文本文件
go run ./cmd/import_domains -file=/path/to/domains.txt
```

**支持格式**：`.xlsx`, `.xls`, `.txt`, `.csv`（文本为每行一个 URL/域名/邮箱）

**自动处理**：清洗 URL、提取一级域名、去重；已存在或写入失败计为「未入库」。

**参数**：`-file`（必填）、`-f`（配置文件，默认 `api/etc/domain.yaml`）

### 3. 手动抢注 (snatch_domain)

对指定域名或抢注任务执行一次抢注：先发飞书「即将抢注」通知，再执行 WHOIS 与 GoDaddy 注册（与 API `POST /api/snatch/execute` 逻辑一致）。

```bash
cd backend

# 按域名抢注（无则自动创建任务）
go run ./cmd/snatch_domain -domain=example.com

# 按任务 ID 抢注
go run ./cmd/snatch_domain -task-id=123
```

**参数**：`-f`（配置文件）、`-domain`（域名）、`-task-id`（任务 ID）；`-domain` 与 `-task-id` 二选一。

### 4. Shell 脚本

```bash
# 快速检查单个域名
./cmd/check.sh baidu.com

# 智能批量检查
./cmd/smart_check.sh
```

## 域名生命周期状态

```
registered → expired → grace_period → redemption → pending_delete → available
```

| 状态 | 说明 |
|------|------|
| `registered` | 已注册 |
| `expired` | 已过期 |
| `grace_period` | 宽限期 |
| `redemption` | 赎回期 |
| `pending_delete` | 待删除 |
| `available` | 可注册 |
| `unknown` | 状态未知 |
