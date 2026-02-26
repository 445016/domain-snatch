# check_domain

手动更新域名 WHOIS 状态的命令行工具。从数据库读取域名，查询 WHOIS 后写回状态、到期时间等字段。

## 运行方式

```bash
go run ./cmd/check_domain [选项]
# 或编译后
./check_domain [选项]
```

## 三种模式

| 模式     | 触发条件                         | 行为                                                                 |
|----------|----------------------------------|----------------------------------------------------------------------|
| 某个域名 | 指定 `-domain=example.com`       | 只对该域名做 WHOIS 并写库                                           |
| 全部域名 | 不指定 `-domain` 且不指定 `-time` | 对库中所有域名（可配合 `-status` / `-limit` / `-desc`）做 WHOIS 并写库 |
| 指定时间 | 不指定 `-domain` 且指定 `-time`   | 只对「当前状态的结束时间 ≤ 指定时间」的域名做 WHOIS 并写库（与 cron 一致） |

## 命令行参数

| 参数        | 类型   | 默认值 | 说明 |
|-------------|--------|--------|------|
| `-f`        | string | api/etc/domain.yaml | 项目配置文件路径，用于读取数据库连接（Mysql.DataSource）。 |
| `-domain`   | string | 空     | 要检查的域名，如 `example.com`。不指定则进入「全部域名」或「指定时间」模式。 |
| `-time`     | string | 空     | 参考时间，仅「指定时间」模式生效。`""` 或 `now` 表示当前时间；也可为 `2006-01-02` 或 `2006-01-02 15:04:05`（日期缺省为 00:00:00）。 |
| `-sleep`    | int    | 2      | 批量模式下，每个域名之间的休眠秒数。 |
| `-status`   | string | 空     | 仅「全部域名」模式生效。只检查指定状态的域名，如：`unknown`、`registered`、`expired`。 |
| `-desc`     | bool   | false  | 仅「全部域名」模式生效。倒序检查（从最新添加的域名开始）。 |
| `-limit`    | int64  | 0      | 仅「全部域名」模式生效。限制检查数量，0 表示不限制。 |

## 使用示例

数据库连接从项目配置文件（默认 `api/etc/domain.yaml`）中的 `Mysql.DataSource` 读取，无需单独指定。

```bash
# 在 backend 目录下执行；使用默认配置 api/etc/domain.yaml
go run ./cmd/check_domain -domain=example.com

# 指定配置文件（如从其他目录运行）
go run ./cmd/check_domain -f=api/etc/domain.yaml -domain=example.com

# 更新「当前状态结束时间 ≤ 当前时间」的域名（与 cron 一致）
go run ./cmd/check_domain -time
go run ./cmd/check_domain -time=now

# 更新「当前状态结束时间 ≤ 2026-02-26 12:00:00」的域名
go run ./cmd/check_domain -time="2026-02-26 12:00:00"

# 更新库里所有域名
go run ./cmd/check_domain

# 只更新状态为 unknown 的域名，最多 100 个，倒序
go run ./cmd/check_domain -status=unknown -limit=100 -desc
```

## 说明

- 指定时间模式：只要命令行出现 `-time` 或 `-time=now` 或 `-time=2026-02-26` 即进入「指定时间」模式；未出现 `-time` 且无 `-domain` 则为「全部域名」模式。
- 按时间模式不叠加 `-status` 等筛选，与 cron 使用的「需更新」查询逻辑一致。
