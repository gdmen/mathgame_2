# installation
- install Node.js
- install Go 1.24.2 (see go.mod)
- install go-swagger (optional; only needed for `make build-docs` / `make dev-docs`)

# config
- Copy `conf.json_` to `conf.json` and fill in MySQL user/pass and any Auth0 or OpenAI keys you need

# install mysql

# set a mysql user/pass
> mysql -u root -p # check path

> USE mysql;

> ALTER USER 'root'@'localhost' IDENTIFIED BY '<PASSWORD>';

> CREATE DATABASE mathgame;

# build (required before first run of dev-api)
> make

# refresh after not developing for a long time
> make clean
> make

# development
Run from repo root. API reads conf.json from current directory.
> make dev-api
> make dev-web

# test
> make test

# production
> see ./deploy/README.md

# insert some test videos
mysql -u root -proot mathgame < deploy/videos.sql

# drop and recreate db
mysql -u root -proot mathgame < deploy/drop.sql
