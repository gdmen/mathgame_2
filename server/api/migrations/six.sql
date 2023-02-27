alter table problems ADD problem_type_bitmap BIGINT UNSIGNED NOT NULL after id;
alter table problems ADD disabled TINYINT NOT NULL DEFAULT 0;
