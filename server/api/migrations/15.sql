alter table users
DROP COLUMN auth0_id
ADD PRIMARY KEY (id)
ADD UNIQUE (email)
RENAME COLUMN username TO name 
ADD password TEXT after email;
