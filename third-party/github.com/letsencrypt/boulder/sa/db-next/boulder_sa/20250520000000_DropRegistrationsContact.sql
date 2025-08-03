-- +migrate Up
-- SQL in section 'Up' is executed when this migration is applied

ALTER TABLE `registrations` DROP COLUMN `contact`;

-- +migrate Down
-- SQL section 'Down' is executed when this migration is rolled back

ALTER TABLE `registrations` ADD COLUMN `contact` varchar(191) CHARACTER SET utf8mb4 DEFAULT '[]';
