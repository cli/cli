-- +migrate Up

DROP TABLE requestedNames;

-- +migrate Down

DROP TABLE requestedNames;

CREATE TABLE `requestedNames` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT,
  `orderID` bigint(20) NOT NULL,
  `reversedName` varchar(253) CHARACTER SET ascii NOT NULL,
  PRIMARY KEY (`id`),
  KEY `orderID_idx` (`orderID`),
  KEY `reversedName_idx` (`reversedName`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
 PARTITION BY RANGE(id)
(PARTITION p_start VALUES LESS THAN (MAXVALUE));
