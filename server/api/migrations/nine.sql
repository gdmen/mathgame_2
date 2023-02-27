alter table videos ADD user_id BIGINT UNSIGNED NOT NULL after id;
--TODO: manually set videos.user_id when migrating in prod
