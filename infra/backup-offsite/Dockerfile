# Off-site backup job: restic (encrypted, deduplicated snapshots) + mc (to
# mirror the object-storage buckets into a local staging cache) + busybox
# crond for the schedule. Binaries are copied from pinned upstream images.
FROM restic/restic:0.19.1@sha256:136600b6ff6843d61d355f7f71f460a166429f35de6fd11b568fece3c9a4d510 AS restic
FROM pgsty/mc:RELEASE.2026-09-16T00-00-00Z@sha256:cfc83108c3abb371f8fb84d99c1fdc88f8c237e022409b0081fb7c0a3be634dd AS mc

FROM alpine:3.24

RUN apk add --no-cache ca-certificates tzdata

COPY --from=restic /usr/bin/restic /usr/local/bin/restic
# `mc` may be a symlink to `mcli` in the fork's image: copy both.
COPY --from=mc /usr/bin/mc* /usr/local/bin/
COPY offsite.sh entrypoint.sh offsite-health /usr/local/bin/
# Strip CR in case the scripts were checked out with Windows line endings.
RUN sed -i 's/\r$//' /usr/local/bin/offsite.sh /usr/local/bin/entrypoint.sh /usr/local/bin/offsite-health \
 && chmod 755 /usr/local/bin/offsite.sh /usr/local/bin/entrypoint.sh /usr/local/bin/offsite-health \
 && restic version && mc --version

ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
