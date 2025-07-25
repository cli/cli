-- +migrate Up
-- SQL in section 'Up' is executed when this migration is applied

CREATE TABLE `replacementOrders` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT,
  `serial` varchar(255) NOT NULL,
  `orderID` bigint(20) NOT NULL,
  `orderExpires` datetime NOT NULL,
  `replaced` boolean DEFAULT false,
  PRIMARY KEY (`id`),
  KEY `serial_idx` (`serial`),
  KEY `orderID_idx` (`orderID`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
 PARTITION BY RANGE(id)
(PARTITION p_start VALUES LESS THAN (MAXVALUE));

-- +migrate Down
-- SQL section 'Down' is executed when this migration is rolled back

DROP TABLE `replacementOrders`;
