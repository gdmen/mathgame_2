alter table settings CHANGE COLUMN target_work_percentage target_work_percentage INT(3) NOT NULL;
--NOTE: manually change settings.target_work_percentage values from double to int when migrating in prod
