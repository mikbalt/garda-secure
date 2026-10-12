# Build stage — Node 22 LTS
FROM node:22.23-alpine AS builder

WORKDIR /app

COPY package.json package-lock.json* ./
RUN npm ci

COPY . .
RUN npm run build

# Production stage — configuration is injected at container start (see
# docker-entrypoint.d/40-securepatrol-config.sh), not baked in at build time.
FROM nginx:1.30-alpine

# config.js is written to /tmp (tmpfs) so the container can run with a
# read-only root filesystem; nginx.conf serves it from there.
ENV API_URL=/api/v1 \
    KEYCLOAK_URL=/auth \
    KEYCLOAK_CLIENT_ID=securepatrol-web \
    KEYCLOAK_REALMS=securepatrol \
    SP_CONFIG_PATH=/tmp/config.js

COPY --from=builder /app/dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY nginx-security-headers.conf /etc/nginx/snippets/web-security-headers.conf
COPY docker-entrypoint.d/40-securepatrol-config.sh /docker-entrypoint.d/40-securepatrol-config.sh
# Strip CR in case the files were checked out with Windows line endings; drop
# the build-time placeholder so only the runtime config can be served.
RUN sed -i 's/\r$//' /docker-entrypoint.d/40-securepatrol-config.sh /etc/nginx/conf.d/default.conf \
      /etc/nginx/snippets/web-security-headers.conf \
 && chmod 755 /docker-entrypoint.d/40-securepatrol-config.sh \
 && rm -f /usr/share/nginx/html/config.js

EXPOSE 80

CMD ["nginx", "-g", "daemon off;"]
