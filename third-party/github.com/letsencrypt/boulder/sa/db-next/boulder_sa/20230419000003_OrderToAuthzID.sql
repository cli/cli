-- +migrate Up
-- SQL in section 'Up' is executed when this migration is applied

DROP TABLE orderToAuthz2;
CREATE TABLE `orderToAuthz2` (
  `id` bigint(20) UNSIGNED NOT NULL AUTO_INCREMENT,
  `orderID` bigint(20) UNSIGNED NOT NULL,
  `authzID` bigint(20) UNSIGNED NOT NULL,
  PRIMARY KEY (`id`),
  KEY `orderID_idx` (`orderID`),
  KEY `authzID_idx` (`authzID`)
) ENGINE=InnoDB AUTO_INCREMENT=9 DEFAULT CHARSET=utf8mb4
 PARTITION BY RANGE (`id`)
(PARTITION p_start VALUES LESS THAN (MAXVALUE));

-- +migrate Down
-- SQL section 'Down' is executed when this migration is rolled back

DROP TABLE orderToAuthz2;
CREATE TABLE `orderToAuthz2` (
  `orderID` bigint(20) NOT NULL,
  `authzID` bigint(20) NOT NULL,
  PRIMARY KEY (`orderID`,`authzID`),
  KEY `authzID` (`authzID`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
 PARTITION BY RANGE COLUMNS(orderID, authzID)
(PARTITION p_start VALUES LESS THAN (MAXVALUE, MAXVALUE));
