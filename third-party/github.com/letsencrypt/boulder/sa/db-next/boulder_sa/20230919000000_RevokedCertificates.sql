-- +migrate Up
-- SQL in section 'Up' is executed when this migration is applied

CREATE TABLE `revokedCertificates` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT,
  `issuerID` bigint(20) NOT NULL,
  `serial` varchar(255) NOT NULL,
  `notAfterHour` datetime NOT NULL,
  `shardIdx` bigint(20) NOT NULL,
  `revokedDate` datetime NOT NULL,
  `revokedReason` int(11) NOT NULL,
  PRIMARY KEY (`id`),
  KEY `issuerID_shardIdx_notAfterHour_idx` (`issuerID`, `shardIdx`, `notAfterHour`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
 PARTITION BY RANGE(id)
(PARTITION p_start VALUES LESS THAN (MAXVALUE));

-- +migrate Down
-- SQL section 'Down' is executed when this migration is rolled back

DROP TABLE `revokedCertificates`;
