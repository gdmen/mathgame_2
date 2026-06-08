# installation
- install Node.js
- install Go 1.24.2 (see go.mod)
- install go-swagger (optional; only needed for `make build-docs` / `make dev-docs`)

# config
- Copy `conf.json_` to `conf.json` and fill in MySQL user/pass and any Auth0 or OpenAI keys you need

# mysql

The app works against any MySQL 8.0+ server (8.4 LTS recommended; 5.7 is
EOL but functional).

## Create the database with the expected charset/collation

The schema uses `utf8mb4` everywhere; the app expects `utf8mb4_unicode_ci`
as the default collation so string sort order is consistent across hosts.
Create the DB explicitly:

```sql
CREATE DATABASE mathgame
  CHARACTER SET utf8mb4
  COLLATE utf8mb4_unicode_ci;
```

For a local install, also create / update the root user if needed:

```sql
ALTER USER 'root'@'localhost' IDENTIFIED BY '<PASSWORD>';
```

## Server config

These server-level defaults minimize surprises if `mysqldump` runs against
the server later or `CREATE TABLE` statements omit explicit charset/collation:

| Variable | Recommended |
|---|---|
| `character_set_server` | `utf8mb4` |
| `collation_server` | `utf8mb4_unicode_ci` |
| `sql_mode` | at least `NO_ENGINE_SUBSTITUTION` (the app tolerates strict mode but isn't required to run with it) |

Check current values with `SHOW VARIABLES LIKE '<name>';`. Where to set
them depends on the host (managed-host config UI, `my.cnf`, etc.).

## Migrations

Schema migrations live in `server/api/migrations/<N>.sql` and are applied
automatically on apiserver startup, in numeric order, recorded in a
`schema_migrations` table. Nothing to run by hand.

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

# drop and recreate db
mysql -u root -proot mathgame < deploy/drop.sql
