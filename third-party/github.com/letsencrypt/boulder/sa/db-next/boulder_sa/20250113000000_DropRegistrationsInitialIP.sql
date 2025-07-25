-- +migrate Up
-- SQL in section 'Up' is executed when this migration is applied

ALTER TABLE `registrations`
DROP COLUMN `initialIP`,
DROP KEY `initialIP_createdAt`;

-- +migrate Down
-- SQL section 'Down' is executed when this migration is rolled back

ALTER TABLE `registrations`
ADD COLUMN `initialIP` binary(16) NOT NULL DEFAULT '\0\0\0\0\0\0\0\0\0\0\0\0\0\0\0\0',
ADD KEY `initialIP_createdAt` (`initialIP`, `createdAt`);
