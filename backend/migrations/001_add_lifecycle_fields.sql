-- 域名生命周期字段迁移
-- 执行方式: mysql -u root -p domain_snatch < migrations/001_add_lifecycle_fields.sql

-- 为 domains 表添加生命周期相关字段
ALTER TABLE domains 
    ADD COLUMN whois_status VARCHAR(500) DEFAULT '' COMMENT 'WHOIS Domain Status 原始值',
    ADD COLUMN delete_date DATETIME DEFAULT NULL COMMENT '预计删除日期';

-- 为 snatch_tasks 表添加自动抢注相关字段
ALTER TABLE snatch_tasks 
    ADD COLUMN auto_register TINYINT DEFAULT 0 COMMENT '是否自动注册 0-否 1-是',
    ADD COLUMN retry_count INT DEFAULT 0 COMMENT '重试次数',
    ADD COLUMN last_error TEXT COMMENT '最后一次错误信息';

-- 添加索引以支持按状态和删除日期查询
ALTER TABLE domains ADD INDEX idx_status_delete_date (status, delete_date);
ALTER TABLE domains ADD INDEX idx_delete_date (delete_date);

-- 更新 status 字段注释
ALTER TABLE domains MODIFY COLUMN status VARCHAR(20) DEFAULT 'unknown' 
    COMMENT '状态: registered/expired/grace_period/redemption/pending_delete/available/unknown';
