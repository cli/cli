
-- +migrate Up
-- SQL in section 'Up' is executed when this migration is applied

ALTER TABLE `certificateStatus` DROP COLUMN `subscriberApproved`;

-- +migrate Down
-- SQL section 'Down' is executed when this migration is rolled back

ALTER TABLE `certificateStatus` ADD COLUMN `subscriberApproved` TINYINT(1) DEFAULT 0;
