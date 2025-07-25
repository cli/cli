-- +migrate Up

DROP TABLE certificatesPerName;
DROP TABLE newOrdersRL;

-- +migrate Down

DROP TABLE certificatesPerName;
DROP TABLE newOrdersRL;

CREATE TABLE `certificatesPerName` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT,
  `eTLDPlusOne` varchar(255) NOT NULL,
  `time` datetime NOT NULL,
  `count` int(11) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `eTLDPlusOne_time_idx` (`eTLDPlusOne`,`time`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE `newOrdersRL` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT,
  `regID` bigint(20) NOT NULL,
  `time` datetime NOT NULL,
  `count` int(11) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `regID_time_idx` (`regID`,`time`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
