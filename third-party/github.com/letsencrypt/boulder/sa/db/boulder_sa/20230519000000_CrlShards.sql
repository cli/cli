-- +migrate Up
-- SQL in section 'Up' is executed when this migration is applied

CREATE TABLE `crlShards` (
  `id` bigint(20) UNSIGNED NOT NULL AUTO_INCREMENT,
  `issuerID` bigint(20) NOT NULL,
  `idx` int UNSIGNED NOT NULL,
  `thisUpdate` datetime,
  `nextUpdate` datetime,
  `leasedUntil` datetime NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `shardID` (`issuerID`, `idx`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- +migrate Down
-- SQL section 'Down' is executed when this migration is rolled back

DROP TABLE `crlShards`;
