#!/bin/sh
set -x
export PATH=/Applications/Docker.app/Contents/Resources/bin:$PATH
docker rm -f pgA pgB >/dev/null 2>&1
docker network rm net72 >/dev/null 2>&1
docker network create net72 >/dev/null
for c in pgA pgB; do
  docker run -d --name $c --network net72 --cap-add=NET_ADMIN --sysctl net.ipv6.conf.all.disable_ipv6=1 alpine:3.20 sleep 86400 >/dev/null
  docker exec $c sh -c 'apk add --no-cache postgresql16 postgresql16-client iproute2 >/dev/null 2>&1'
  # 40ms egress each side => ~80ms RTT, like Beijing<->Singapore cross-region link
  docker exec $c tc qdisc add dev eth0 root netem delay 40ms
done
docker exec pgA sh -c 'mkdir -p /var/lib/postgresql/data /run/postgresql && chown -R postgres:postgres /var/lib/postgresql /run/postgresql'
docker exec -u postgres pgA sh -c 'initdb -D /var/lib/postgresql/data -A trust -U postgres' >/dev/null
cat >> /dev/null <<'X'
X
docker exec -u postgres pgA sh -c "cat >> /var/lib/postgresql/data/postgresql.conf <<'CONF'
listen_addresses = '*'
wal_level = replica
max_wal_senders = 10
hot_standby = on
synchronous_commit = on
CONF"
docker exec -u postgres pgA sh -c "cat > /var/lib/postgresql/data/pg_hba.conf <<'HBA'
local all all trust
host all all all trust
host replication all all trust
HBA"
docker exec -u postgres pgA sh -c 'pg_ctl -D /var/lib/postgresql/data -l /tmp/pg.log -o "-c listen_addresses=*" start' 
sleep 2
docker exec -u postgres pgB sh -c 'mkdir -p /var/lib/postgresql/data /run/postgresql && chown -R postgres:postgres /var/lib/postgresql /run/postgresql'
docker exec -u postgres pgB sh -c 'pg_basebackup -h pgA -U postgres -D /var/lib/postgresql/data -Fp -Xs -P -R' 2>&1 | tail -3
docker exec -u postgres pgB sh -c 'pg_ctl -D /var/lib/postgresql/data -l /tmp/pg.log -o "-c listen_addresses=*" start'
sleep 3
echo "=== A ==="; docker exec -u postgres pgA sh -c 'psql -Atc "select pg_is_in_recovery(), current_setting('"'"'synchronous_commit'"'"')"'
echo "=== B ==="; docker exec -u postgres pgB sh -c 'psql -Atc "select pg_is_in_recovery(), pg_last_wal_replay_lsn() is not null"'
echo "=== RTT A->B (pg ping via psql \\timing) ==="
docker exec -u postgres pgA sh -c 'psql -h pgB -U postgres -Atc "select 1"' && echo NET_OK
