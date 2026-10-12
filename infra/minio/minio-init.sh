#!/bin/sh
# One-shot object-storage setup (service minio-init). Idempotent.
#  - buckets for photos and backups, both versioned (evidence photos cannot be
#    silently replaced; dumps keep history) — BE-S4 / INF-B5
#  - lifecycle on the backups bucket: dumps expire after BACKUP_RETENTION_DAYS,
#    old versions 7 days later (off-site copies are kept longer by restic)
#  - opt-in least-privilege users (INF-S5): an app user limited to the two
#    buckets and a read-only off-site user, when their keys are set in .env
# Root credentials come from MC_HOST_sp (environment). Only uses shell
# builtins + mc (the image is ubi-micro: no sed/awk).
set -eu

: "${MINIO_BUCKET_PHOTOS:?}" "${MINIO_BUCKET_BACKUPS:?}"
days="${BACKUP_RETENTION_DAYS:-35}"
case "$days" in ''|*[!0-9]*) echo "BACKUP_RETENTION_DAYS must be a number" >&2; exit 1 ;; esac

for b in "$MINIO_BUCKET_PHOTOS" "$MINIO_BUCKET_BACKUPS"; do
  mc mb --ignore-existing "sp/$b"
  mc version enable "sp/$b"
done

cat > /tmp/lifecycle.json <<EOF
{
  "Rules": [
    {
      "ID": "securepatrol-backup-retention",
      "Status": "Enabled",
      "Filter": { "Prefix": "" },
      "Expiration": { "Days": $days },
      "NoncurrentVersionExpiration": { "NoncurrentDays": 7 }
    }
  ]
}
EOF
# `import` replaces the whole lifecycle configuration: idempotent.
mc ilm rule import "sp/$MINIO_BUCKET_BACKUPS" < /tmp/lifecycle.json

policy_file() { # $1 name, $2 bucket-level actions, $3 object-level actions (JSON arrays)
  cat > "/tmp/$1.json" <<EOF
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": $2,
      "Resource": ["arn:aws:s3:::$MINIO_BUCKET_PHOTOS", "arn:aws:s3:::$MINIO_BUCKET_BACKUPS"]
    },
    {
      "Effect": "Allow",
      "Action": $3,
      "Resource": ["arn:aws:s3:::$MINIO_BUCKET_PHOTOS/*", "arn:aws:s3:::$MINIO_BUCKET_BACKUPS/*"]
    }
  ]
}
EOF
}

ensure_user() { # $1 access key, $2 secret key, $3 policy name
  # `user add` also updates the secret of an existing user.
  mc admin user add sp "$1" "$2" >/dev/null
  mc admin policy attach sp "$3" --user "$1" >/dev/null 2>&1 || true
  echo "minio-init: user $1 -> policy $3"
}

if [ -n "${MINIO_APP_ACCESS_KEY:-}" ] && [ -n "${MINIO_APP_SECRET_KEY:-}" ]; then
  # The API creates/versions the buckets on start (EnsureBuckets), uploads
  # photos and dumps, and pre-signs reads. Deletes only on the photos bucket
  # (retention job and floor-plan removal delete every version, BE-S9/F3);
  # backups can never be deleted by the app.
  # Written in full (the image has no sed/awk to extend policy_file output).
  cat > /tmp/securepatrol-app.json <<EOF
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": ["s3:GetBucketLocation", "s3:ListBucket", "s3:ListBucketMultipartUploads", "s3:GetBucketVersioning", "s3:PutBucketVersioning", "s3:CreateBucket"],
      "Resource": ["arn:aws:s3:::$MINIO_BUCKET_PHOTOS", "arn:aws:s3:::$MINIO_BUCKET_BACKUPS"]
    },
    {
      "Effect": "Allow",
      "Action": ["s3:GetObject", "s3:PutObject", "s3:AbortMultipartUpload", "s3:ListMultipartUploadParts"],
      "Resource": ["arn:aws:s3:::$MINIO_BUCKET_PHOTOS/*", "arn:aws:s3:::$MINIO_BUCKET_BACKUPS/*"]
    },
    {
      "Effect": "Allow",
      "Action": ["s3:ListBucketVersions"],
      "Resource": ["arn:aws:s3:::$MINIO_BUCKET_PHOTOS"]
    },
    {
      "Effect": "Allow",
      "Action": ["s3:DeleteObject", "s3:DeleteObjectVersion"],
      "Resource": ["arn:aws:s3:::$MINIO_BUCKET_PHOTOS/*"]
    }
  ]
}
EOF
  mc admin policy create sp securepatrol-app /tmp/securepatrol-app.json
  ensure_user "$MINIO_APP_ACCESS_KEY" "$MINIO_APP_SECRET_KEY" securepatrol-app
fi

if [ -n "${MINIO_OFFSITE_ACCESS_KEY:-}" ] && [ -n "${MINIO_OFFSITE_SECRET_KEY:-}" ]; then
  # Read-only: list + read objects (and their tags) for `mc mirror`.
  policy_file securepatrol-offsite \
    '["s3:GetBucketLocation", "s3:ListBucket", "s3:GetBucketVersioning"]' \
    '["s3:GetObject", "s3:GetObjectTagging"]'
  mc admin policy create sp securepatrol-offsite /tmp/securepatrol-offsite.json
  ensure_user "$MINIO_OFFSITE_ACCESS_KEY" "$MINIO_OFFSITE_SECRET_KEY" securepatrol-offsite
fi

echo "minio-init: done (buckets versioned, backups expire after ${days} days)"
