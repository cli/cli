-- +migrate Up
-- SQL in section 'Up' is executed when this migration is applied

CREATE TABLE overrides (
   `limitEnum` tinyint(4) UNSIGNED NOT NULL,
   `bucketKey` varchar(255) NOT NULL,
   `comment`   varchar(255) NOT NULL,
   `periodNS`  bigint(20) UNSIGNED NOT NULL,
   `count`     int UNSIGNED NOT NULL,
   `burst`     int UNSIGNED NOT NULL,
   `updatedAt` datetime NOT NULL,
   `enabled`   boolean NOT NULL DEFAULT false,
  UNIQUE KEY `limitEnum_bucketKey` (`limitEnum`, `bucketKey`),
  INDEX idx_enabled (enabled)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- +migrate Down
-- SQL section 'Down' is executed when this migration is rolled back

DROP TABLE IF EXISTS overrides;
