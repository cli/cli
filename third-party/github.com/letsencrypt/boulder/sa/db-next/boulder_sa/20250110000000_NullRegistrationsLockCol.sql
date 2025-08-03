
-- +migrate Up
-- SQL in section 'Up' is executed when this migration is applied

ALTER TABLE `registrations` ALTER COLUMN `LockCol` SET DEFAULT 0;

-- +migrate Down
-- SQL section 'Down' is executed when this migration is rolled back

ALTER TABLE `registrations` ALTER COLUMN `LockCol` DROP DEFAULT;
