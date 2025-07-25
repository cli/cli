-- this file is run by test/create_db.sh to create users for each
-- component with the appropriate permissions.

-- These lines require MariaDB 10.1+
CREATE USER IF NOT EXISTS 'incidents_sa'@'localhost';
CREATE USER IF NOT EXISTS 'test_setup'@'localhost';

-- Storage Authority
GRANT SELECT ON * TO 'incidents_sa'@'localhost';

-- Test setup and teardown
GRANT ALL PRIVILEGES ON * to 'test_setup'@'localhost';
