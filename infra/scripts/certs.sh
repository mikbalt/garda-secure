#!/bin/sh
# Private CA + server certificate for SERVER_HOST (INF-D3).
#   scripts/certs.sh [SERVER_HOST]        (default: SERVER_HOST from infra/.env)
#   scripts/certs.sh --renew [SERVER_HOST] new server cert from the existing CA
#
# Creates in infra/nginx/certs/:
#   ca.pem / ca-key.pem   private CA (10 years). Install ca.pem — NOT cert.pem —
#                         on every tablet and admin PC once. The CA is name-
#                         constrained to SERVER_HOST + localhost, so it cannot
#                         be abused to impersonate other sites.
#   securepatrol-ca.crt   copy of ca.pem for Android ("Install certificate")
#   cert.pem / key.pem    server certificate (825 days) used by nginx; SAN =
#                         SERVER_HOST (DNS or IP) + localhost + 127.0.0.1.
# Using a public/ACME certificate instead: see docs/runbook.md §6.
set -eu
. "$(dirname -- "$0")/lib.sh"

renew=0
if [ "${1:-}" = "--renew" ]; then renew=1; shift; fi
host=${1:-}
if [ -z "$host" ]; then
  [ -f "$INFRA_DIR/.env" ] && load_env "$INFRA_DIR/.env"
  host=${SERVER_HOST:-}
fi
[ -n "$host" ] && [ "$host" != change_me ] || die "usage: $0 [--renew] <SERVER_HOST> (or set SERVER_HOST in infra/.env)"
command -v openssl >/dev/null || die "openssl is required"

if is_ipv4 "$host"; then
  san_host="IP:$host"
  nc_host="IP:$host/255.255.255.255"
elif is_hostname "$host"; then
  san_host="DNS:$host"
  nc_host="DNS:$host"
else
  die "SERVER_HOST must be a DNS name or an IPv4 address: $host"
fi
if [ "$host" = localhost ]; then
  san="DNS:localhost,IP:127.0.0.1"
  nc="permitted;DNS:localhost,permitted;IP:127.0.0.1/255.255.255.255"
else
  san="$san_host,DNS:localhost,IP:127.0.0.1"
  nc="permitted;$nc_host,permitted;DNS:localhost,permitted;IP:127.0.0.1/255.255.255.255"
fi

dir="$INFRA_DIR/nginx/certs"
mkdir -p "$dir"
chmod 700 "$dir"
umask 077
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

if [ -f "$dir/ca.pem" ] && [ -f "$dir/ca-key.pem" ]; then
  info "using existing CA $dir/ca.pem"
  # The CA's name constraints must cover the host.
  if ! openssl x509 -in "$dir/ca.pem" -noout -ext nameConstraints 2>/dev/null | grep -q "${nc_host#*:}"; then
    die "the existing CA is not valid for $host (name constraints). Move nginx/certs/ca*.pem away to create a new CA — every device must then trust the new ca.pem."
  fi
else
  [ "$renew" = 1 ] && die "--renew needs an existing CA in $dir"
  if [ -f "$dir/cert.pem" ]; then
    die "$dir/cert.pem exists but no private CA: it is probably a public/ACME certificate. Remove it to switch to a private CA."
  fi
  info "creating private CA (name-constrained to $host)"
  cat > "$work/ca.cnf" <<EOF
[req]
distinguished_name = dn
prompt = no
x509_extensions = v3_ca
[dn]
O = SecurePatrol
CN = SecurePatrol Private CA ($host)
[v3_ca]
basicConstraints = critical,CA:TRUE,pathlen:0
keyUsage = critical,keyCertSign,cRLSign
subjectKeyIdentifier = hash
nameConstraints = critical,$nc
EOF
  openssl req -x509 -new -nodes -newkey rsa:3072 -sha256 -days 3650 \
    -keyout "$dir/ca-key.pem" -out "$dir/ca.pem" -config "$work/ca.cnf"
fi

info "issuing server certificate for $host"
cat > "$work/server.cnf" <<EOF
[req]
distinguished_name = dn
prompt = no
[dn]
O = SecurePatrol
CN = $host
EOF
cat > "$work/server.ext" <<EOF
basicConstraints = critical,CA:FALSE
keyUsage = critical,digitalSignature,keyEncipherment
extendedKeyUsage = serverAuth
subjectKeyIdentifier = hash
authorityKeyIdentifier = keyid,issuer
subjectAltName = $san
EOF
openssl req -new -nodes -newkey rsa:2048 -sha256 \
  -keyout "$work/key.pem" -out "$work/server.csr" -config "$work/server.cnf"
openssl x509 -req -in "$work/server.csr" -sha256 -days 825 \
  -CA "$dir/ca.pem" -CAkey "$dir/ca-key.pem" -CAcreateserial -CAserial "$dir/ca.srl" \
  -extfile "$work/server.ext" -out "$work/cert.pem"
openssl verify -CAfile "$dir/ca.pem" "$work/cert.pem" >/dev/null

mv "$work/key.pem" "$dir/key.pem"
mv "$work/cert.pem" "$dir/cert.pem"
cp "$dir/ca.pem" "$dir/securepatrol-ca.crt"
chmod 600 "$dir/key.pem" "$dir/ca-key.pem"
# nginx (root master) and the blackbox exporter read these.
chmod 644 "$dir/cert.pem" "$dir/ca.pem" "$dir/securepatrol-ca.crt"
chmod 755 "$dir"

echo
info "certificate: $(openssl x509 -in "$dir/cert.pem" -noout -subject -enddate | tr '\n' ' ')"
echo "    SAN: $san"
echo "    Install the CA (nginx/certs/securepatrol-ca.crt) once on every tablet and"
echo "    admin PC — see docs/runbook.md §6. Reload nginx after renewing:"
echo "    docker compose exec nginx nginx -s reload"
