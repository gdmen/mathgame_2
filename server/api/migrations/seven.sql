alter table videos DROP COLUMN start;
alter table videos DROP COLUMN end;
alter table videos CHANGE COLUMN thumbnailurl thumbnailurl VARCHAR(256) NOT NULL AFTER url;
alter table videos CHANGE enabled disabled TINYINT NOT NULL DEFAULT 0;
update videos SET disabled = 0;
