-- Database per service: no service can read another's tables.
-- (Services also create their database on start-up if it is missing,
--  so new services work against an existing volume.)
CREATE DATABASE auth_db;
CREATE DATABASE master_db;
CREATE DATABASE order_db;
CREATE DATABASE warehouse_db;
CREATE DATABASE billing_db;
