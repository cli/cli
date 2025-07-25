-- +migrate Up
-- SQL in section 'Up' is executed when this migration is applied

-- This table has no auto-incrementing primary key because we don't plan to
-- partition it. This table expected to be < 800K rows initially and grow at a
-- rate of ~18% per year.

CREATE TABLE `paused` (
  `registrationID` bigint(20) UNSIGNED NOT NULL,
  `identifierType` tinyint(4) NOT NULL,
  `identifierValue` varchar(255) NOT NULL,
  `pausedAt` datetime NOT NULL,
  `unpausedAt` datetime DEFAULT NULL,
  PRIMARY KEY (`registrationID`, `identifierValue`, `identifierType`)
);

-- +migrate Down
-- SQL section 'Down' is executed when this migration is rolled back

DROP TABLE `paused`;
