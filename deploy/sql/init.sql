-- Domain Snatch Platform Database Init
CREATE DATABASE IF NOT EXISTS domain_snatch DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;

USE domain_snatch;

-- 用户表
CREATE TABLE IF NOT EXISTS `users` (
    `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    `username` VARCHAR(64) NOT NULL DEFAULT '',
    `password_hash` VARCHAR(255) NOT NULL DEFAULT '',
    `role` VARCHAR(16) NOT NULL DEFAULT 'user' COMMENT 'admin/user',
    `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    `updated_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`),
    UNIQUE KEY `idx_username` (`username`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='用户表';

-- 域名表
CREATE TABLE IF NOT EXISTS `domains` (
    `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    `domain` VARCHAR(255) NOT NULL DEFAULT '',
    `status` VARCHAR(32) NOT NULL DEFAULT 'unknown' COMMENT 'registered/expired/available/unknown',
    `expiry_date` DATETIME DEFAULT NULL,
    `creation_date` DATETIME DEFAULT NULL,
    `registrar` VARCHAR(255) NOT NULL DEFAULT '',
    `whois_raw` TEXT,
    `monitor` TINYINT(1) NOT NULL DEFAULT 0 COMMENT '是否监控 0-否 1-是',
    `last_checked` DATETIME DEFAULT NULL,
    `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    `updated_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`),
    UNIQUE KEY `idx_domain` (`domain`),
    KEY `idx_status` (`status`),
    KEY `idx_monitor` (`monitor`),
    KEY `idx_expiry_date` (`expiry_date`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='域名表';

-- 抢注任务表
CREATE TABLE IF NOT EXISTS `snatch_tasks` (
    `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    `domain_id` BIGINT UNSIGNED NOT NULL DEFAULT 0,
    `domain` VARCHAR(255) NOT NULL DEFAULT '',
    `status` VARCHAR(32) NOT NULL DEFAULT 'pending' COMMENT 'pending/processing/success/failed',
    `priority` INT NOT NULL DEFAULT 0 COMMENT '优先级，越大越高',
    `target_registrar` VARCHAR(255) NOT NULL DEFAULT '',
    `result` TEXT,
    `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    `updated_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`),
    KEY `idx_domain_id` (`domain_id`),
    KEY `idx_status` (`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='抢注任务表';

-- 通知日志表
CREATE TABLE IF NOT EXISTS `notify_logs` (
    `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    `domain_id` BIGINT UNSIGNED NOT NULL DEFAULT 0,
    `domain` VARCHAR(255) NOT NULL DEFAULT '',
    `notify_type` VARCHAR(32) NOT NULL DEFAULT '' COMMENT 'expire_warning/available/snatch_result',
    `channel` VARCHAR(16) NOT NULL DEFAULT 'feishu',
    `content` TEXT,
    `status` VARCHAR(16) NOT NULL DEFAULT 'sent' COMMENT 'sent/failed',
    `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`),
    KEY `idx_domain_id` (`domain_id`),
    KEY `idx_notify_type` (`notify_type`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='通知日志表';

-- 通知设置表
CREATE TABLE IF NOT EXISTS `notify_settings` (
    `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    `webhook_url` VARCHAR(512) NOT NULL DEFAULT '',
    `expire_days` INT NOT NULL DEFAULT 30 COMMENT '提前N天提醒',
    `enabled` TINYINT(1) NOT NULL DEFAULT 1,
    `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    `updated_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='通知设置表';

-- 插入默认管理员账号 (密码: admin123)
INSERT INTO `users` (`username`, `password_hash`, `role`) VALUES
('admin', '$2a$10$ybIg8NpXQbaxRHoij8RXNu5XktSw3CDP0wPV6bPr1l/BOiuzscEUu', 'admin');

-- 插入默认通知设置
INSERT INTO `notify_settings` (`webhook_url`, `expire_days`, `enabled`) VALUES
('', 30, 0);
