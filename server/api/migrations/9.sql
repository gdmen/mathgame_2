alter table videos ADD user_id BIGINT UNSIGNED NOT NULL after id;
--NOTE: manually set videos.user_id when migrating in prod
