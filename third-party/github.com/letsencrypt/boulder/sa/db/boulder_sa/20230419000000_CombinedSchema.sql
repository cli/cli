-- +migrate Up
-- SQL in section 'Up' is executed when this migration is applied

CREATE TABLE `authz2` (
  `id` bigint(20) UNSIGNED NOT NULL AUTO_INCREMENT,
  `identifierType` tinyint(4) NOT NULL,
  `identifierValue` varchar(255) NOT NULL,
  `registrationID` bigint(20) NOT NULL,
  `status` tinyint(4) NOT NULL,
  `expires` datetime NOT NULL,
  `challenges` tinyint(4) NOT NULL,
  `attempted` tinyint(4) DEFAULT NULL,
  `attemptedAt` datetime DEFAULT NULL,
  `token` binary(32) NOT NULL,
  `validationError` mediumblob DEFAULT NULL,
  `validationRecord` mediumblob DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `regID_expires_idx` (`registrationID`,`status`,`expires`),
  KEY `regID_identifier_status_expires_idx` (`registrationID`,`identifierType`,`identifierValue`,`status`,`expires`),
  KEY `expires_idx` (`expires`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
 PARTITION BY RANGE(id)
(PARTITION p_start VALUES LESS THAN (MAXVALUE));

CREATE TABLE `blockedKeys` (
  `id` bigint(20) UNSIGNED NOT NULL AUTO_INCREMENT,
  `keyHash` binary(32) NOT NULL,
  `added` datetime NOT NULL,
  `source` tinyint(4) NOT NULL,
  `comment` varchar(255) DEFAULT NULL,
  `revokedBy` bigint(20) DEFAULT 0,
  `extantCertificatesChecked` tinyint(1) DEFAULT 0,
  PRIMARY KEY (`id`),
  UNIQUE KEY `keyHash` (`keyHash`),
  KEY `extantCertificatesChecked_idx` (`extantCertificatesChecked`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE `certificateStatus` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT,
  `serial` varchar(255) NOT NULL,
  `subscriberApproved` tinyint(1) DEFAULT 0,
  `status` varchar(255) NOT NULL,
  `ocspLastUpdated` datetime NOT NULL,
  `revokedDate` datetime NOT NULL,
  `revokedReason` int(11) NOT NULL,
  `lastExpirationNagSent` datetime NOT NULL,
  `LockCol` bigint(20) DEFAULT 0,
  `ocspResponse` blob DEFAULT NULL,
  `notAfter` datetime DEFAULT NULL,
  `isExpired` tinyint(1) DEFAULT 0,
  `issuerID` bigint(20) DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `serial` (`serial`),
  KEY `isExpired_ocspLastUpdated_idx` (`isExpired`,`ocspLastUpdated`),
  KEY `notAfter_idx` (`notAfter`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
 PARTITION BY RANGE(id)
(PARTITION p_start VALUES LESS THAN (MAXVALUE));

CREATE TABLE `certificates` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT,
  `registrationID` bigint(20) NOT NULL,
  `serial` varchar(255) NOT NULL,
  `digest` varchar(255) NOT NULL,
  `der` mediumblob NOT NULL,
  `issued` datetime NOT NULL,
  `expires` datetime NOT NULL,
  PRIMARY KEY (`id`),
  KEY `serial` (`serial`),
  KEY `regId_certificates_idx` (`registrationID`) COMMENT 'Common lookup',
  KEY `issued_idx` (`issued`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
 PARTITION BY RANGE(id)
(PARTITION p_start VALUES LESS THAN (MAXVALUE));

CREATE TABLE `certificatesPerName` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT,
  `eTLDPlusOne` varchar(255) NOT NULL,
  `time` datetime NOT NULL,
  `count` int(11) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `eTLDPlusOne_time_idx` (`eTLDPlusOne`,`time`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE `fqdnSets` (
  `id` bigint(20) UNSIGNED NOT NULL AUTO_INCREMENT,
  `setHash` binary(32) NOT NULL,
  `serial` varchar(255) NOT NULL,
  `issued` datetime NOT NULL,
  `expires` datetime NOT NULL,
  PRIMARY KEY (`id`),
  KEY `serial` (`serial`),
  KEY `setHash_issued_idx` (`setHash`,`issued`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
 PARTITION BY RANGE(id)
(PARTITION p_start VALUES LESS THAN (MAXVALUE));

CREATE TABLE `incidents` (
  `id` bigint(20) UNSIGNED NOT NULL AUTO_INCREMENT,
  `serialTable` varchar(128) NOT NULL,
  `url` varchar(1024) NOT NULL,
  `renewBy` datetime NOT NULL,
  `enabled` boolean DEFAULT false,
  PRIMARY KEY (`id`)
) CHARSET=utf8mb4;

CREATE TABLE `issuedNames` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT,
  `reversedName` varchar(640) CHARACTER SET ascii NOT NULL,
  `notBefore` datetime NOT NULL,
  `serial` varchar(255) NOT NULL,
  `renewal` tinyint(1) NOT NULL DEFAULT 0,
  PRIMARY KEY (`id`),
  KEY `reversedName_notBefore_Idx` (`reversedName`,`notBefore`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
 PARTITION BY RANGE(id)
(PARTITION p_start VALUES LESS THAN (MAXVALUE));

CREATE TABLE `keyHashToSerial` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT,
  `keyHash` binary(32) NOT NULL,
  `certNotAfter` datetime NOT NULL,
  `certSerial` varchar(255) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `unique_keyHash_certserial` (`keyHash`,`certSerial`),
  KEY `keyHash_certNotAfter` (`keyHash`,`certNotAfter`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE `newOrdersRL` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT,
  `regID` bigint(20) NOT NULL,
  `time` datetime NOT NULL,
  `count` int(11) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `regID_time_idx` (`regID`,`time`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE `orderFqdnSets` (
  `id` bigint(20) UNSIGNED NOT NULL AUTO_INCREMENT,
  `setHash` binary(32) NOT NULL,
  `orderID` bigint(20) NOT NULL,
  `registrationID` bigint(20) NOT NULL,
  `expires` datetime NOT NULL,
  PRIMARY KEY (`id`),
  KEY `setHash_expires_idx` (`setHash`,`expires`),
  KEY `orderID_idx` (`orderID`),
  KEY `orderFqdnSets_registrationID_registrations` (`registrationID`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
 PARTITION BY RANGE(id)
(PARTITION p_start VALUES LESS THAN (MAXVALUE));

CREATE TABLE `orderToAuthz2` (
  `orderID` bigint(20) NOT NULL,
  `authzID` bigint(20) NOT NULL,
  PRIMARY KEY (`orderID`,`authzID`),
  KEY `authzID` (`authzID`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
 PARTITION BY RANGE COLUMNS(orderID, authzID)
(PARTITION p_start VALUES LESS THAN (MAXVALUE, MAXVALUE));

CREATE TABLE `orders` (
  `id` bigint(20) UNSIGNED NOT NULL AUTO_INCREMENT,
  `registrationID` bigint(20) NOT NULL,
  `expires` datetime NOT NULL,
  `error` mediumblob DEFAULT NULL,
  `certificateSerial` varchar(255) DEFAULT NULL,
  `beganProcessing` tinyint(1) NOT NULL DEFAULT 0,
  `created` datetime NOT NULL,
  PRIMARY KEY (`id`),
  KEY `reg_status_expires` (`registrationID`,`expires`),
  KEY `regID_created_idx` (`registrationID`,`created`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
 PARTITION BY RANGE(id)
(PARTITION p_start VALUES LESS THAN (MAXVALUE));

CREATE TABLE `precertificates` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT,
  `registrationID` bigint(20) NOT NULL,
  `serial` varchar(255) NOT NULL,
  `der` mediumblob NOT NULL,
  `issued` datetime NOT NULL,
  `expires` datetime NOT NULL,
  PRIMARY KEY (`id`),
  KEY `serial` (`serial`),
  KEY `regId_precertificates_idx` (`registrationID`),
  KEY `issued_precertificates_idx` (`issued`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
 PARTITION BY RANGE(id)
(PARTITION p_start VALUES LESS THAN (MAXVALUE));

CREATE TABLE `registrations` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT,
  `jwk` mediumblob NOT NULL,
  `jwk_sha256` varchar(255) NOT NULL,
  `contact` varchar(191) CHARACTER SET utf8mb4 NOT NULL,
  `agreement` varchar(255) NOT NULL,
  `LockCol` bigint(20) NOT NULL,
  `initialIP` binary(16) NOT NULL DEFAULT '\0\0\0\0\0\0\0\0\0\0\0\0\0\0\0\0',
  `createdAt` datetime NOT NULL,
  `status` varchar(255) NOT NULL DEFAULT 'valid',
  PRIMARY KEY (`id`),
  UNIQUE KEY `jwk_sha256` (`jwk_sha256`),
  KEY `initialIP_createdAt` (`initialIP`,`createdAt`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

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

-- Tables below have foreign key constraints, so are created after all other tables.

CREATE TABLE `serials` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT,
  `registrationID` bigint(20) NOT NULL,
  `serial` varchar(255) NOT NULL,
  `created` datetime NOT NULL,
  `expires` datetime NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `serial` (`serial`),
  KEY `regId_serials_idx` (`registrationID`),
  CONSTRAINT `regId_serials` FOREIGN KEY (`registrationID`) REFERENCES `registrations` (`id`) ON DELETE NO ACTION ON UPDATE NO ACTION
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- +migrate Down
-- SQL section 'Down' is executed when this migration is rolled back

-- First set of tables have foreign key constraints, so are dropped first.
DROP TABLE `serials`;

DROP TABLE `authz2`;
DROP TABLE `blockedKeys`;
DROP TABLE `certificateStatus`;
DROP TABLE `certificatesPerName`;
DROP TABLE `certificates`;
DROP TABLE `fqdnSets`;
DROP TABLE `issuedNames`;
DROP TABLE `keyHashToSerial`;
DROP TABLE `newOrdersRL`;
DROP TABLE `orderFqdnSets`;
DROP TABLE `orderToAuthz2`;
DROP TABLE `orders`;
DROP TABLE `precertificates`;
DROP TABLE `registrations`;
DROP TABLE `requestedNames`;
