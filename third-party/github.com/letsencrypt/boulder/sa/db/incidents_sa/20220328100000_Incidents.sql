-- +migrate Up
-- SQL in section 'Up' is executed when this migration is applied

CREATE TABLE `incident_foo` (
    `serial` varchar(255) NOT NULL,
    `registrationID` bigint(20) unsigned NULL,
    `orderID` bigint(20) unsigned NULL,
    `lastNoticeSent` datetime NULL,
    PRIMARY KEY (`serial`),
    KEY `registrationID_idx` (`registrationID`),
    KEY `orderID_idx` (`orderID`)
) CHARSET=utf8mb4;

CREATE TABLE `incident_bar` (
    `serial` varchar(255) NOT NULL,
    `registrationID` bigint(20) unsigned NULL,
    `orderID` bigint(20) unsigned NULL,
    `lastNoticeSent` datetime NULL,
    PRIMARY KEY (`serial`),
    KEY `registrationID_idx` (`registrationID`),
    KEY `orderID_idx` (`orderID`)
) CHARSET=utf8mb4;

-- +migrate Down
-- SQL section 'Down' is executed when this migration is rolled back

DROP TABLE `incident_foo`;
DROP TABLE `incident_bar`;
