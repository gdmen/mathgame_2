# installation
- install node.js
- install go
- install go-swagger

# config
- conf.json_ and test_conf.json_ -> fill them out and remove the trailing underscore

# install mysql

# set a mysql user/pass
> mysql -u root -h 127.0.0.1 -p # check path

> USE mysql;

> ALTER USER 'root'@'localhost' IDENTIFIED BY '<PASSWORD>';

> CREATE DATABASE mathgame;

# refresh after not devloping for a long time
> make clean  
> make # should install go dependencies
> 
