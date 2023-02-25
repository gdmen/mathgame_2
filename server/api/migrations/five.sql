alter table settings ADD problem_type_bitmap BIGINT UNSIGNED NOT NULL DEFAULT 0 after user_id;
alter table settings DROP COLUMN operations;
alter table settings DROP COLUMN fractions;
alter table settings DROP COLUMN negatives;
