#!/bin/sh
# Run by the official nginx entrypoint before nginx starts (and before
# `nginx -t` in CI). Generates the environment-specific includes:
#   /etc/nginx/generated/admin-allow.conf     from ADMIN_ALLOW_CIDRS
#   /etc/nginx/generated/photos-location.conf from MINIO_BUCKET_PHOTOS
# Values are validated, so .env cannot inject nginx directives.
set -eu

dir=/etc/nginx/generated
mkdir -p "$dir"

cidrs="${ADMIN_ALLOW_CIDRS:-10.0.0.0/8 172.16.0.0/12 192.168.0.0/16 127.0.0.1/32}"
tmp="$dir/admin-allow.conf.tmp"
: > "$tmp"
for c in $cidrs; do
  case "$c" in
    *[!0-9a-fA-F:./]*|'') echo "40-securepatrol: invalid entry in ADMIN_ALLOW_CIDRS: $c" >&2; exit 1 ;;
  esac
  echo "allow $c;" >> "$tmp"
done
echo "deny all;" >> "$tmp"
mv "$tmp" "$dir/admin-allow.conf"

bucket="${MINIO_BUCKET_PHOTOS:-securepatrol-photos}"
case "$bucket" in
  *[!a-z0-9.-]*|''|.*|-*) echo "40-securepatrol: invalid MINIO_BUCKET_PHOTOS: $bucket" >&2; exit 1 ;;
esac
cat > "$dir/photos-location.conf" <<EOF
location ^~ /$bucket/ {
    include /etc/nginx/snippets/photos-proxy.conf;
}
EOF

echo "40-securepatrol: admin allow-list [$cidrs], photos bucket /$bucket/"
