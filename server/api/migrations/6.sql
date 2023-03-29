alter table problems ADD problem_type_bitmap BIGINT UNSIGNED NOT NULL after id;
update problems set problem_type_bitmap=3;
update problems set problem_type_bitmap=1 where expression NOT LIKE '%-%';
update problems set problem_type_bitmap=2 where expression NOT LIKE '%+%';
alter table problems ADD disabled TINYINT NOT NULL DEFAULT 0;
