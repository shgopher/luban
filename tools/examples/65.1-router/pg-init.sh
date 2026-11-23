set -x
apk add --no-cache postgresql16 || exit 1
mkdir -p /pg && chown postgres /pg
mkdir -p /run/postgresql && chown postgres:postgres /run/postgresql
su postgres -c 'initdb -D /pg/data' || exit 1
su postgres -c 'pg_ctl -D /pg/data -o "-p 5432 -c listen_addresses=* -c max_connections=100" -l /pg/log start' || exit 1
sleep 2
su postgres -c "psql -c \"ALTER USER postgres PASSWORD 'pg'\"" || exit 1
tail -f /dev/null
