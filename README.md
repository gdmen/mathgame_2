# installation
- install node.js
- install go
- install go-swagger

# config
- Fill out conf.json_ and remove the trailing underscore

# install mysql

# set a mysql user/pass
> mysql -u root -p # check path

> USE mysql;

> ALTER USER 'root'@'localhost' IDENTIFIED BY '<PASSWORD>';

> CREATE DATABASE mathgame;

# refresh after not developing for a long time
> make clean  
> make

# development
> make dev_api
> make dev_web

# production
> see ./deploy/README.md

# insert some test videos
mysql -u root -proot mathgame < videos.sql

# drop and recreate db
mysql -u root -proot mathgame < drop.sql
