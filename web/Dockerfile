# Build stage
FROM node:20-alpine AS builder

WORKDIR /app

COPY package.json package-lock.json* ./
RUN npm ci

COPY . .
RUN npm run build

# Production stage — configuration is injected at container start (see
# docker-entrypoint.d/40-securepatrol-config.sh), not baked in at build time.
FROM nginx:alpine

ENV API_URL=/api/v1 \
    KEYCLOAK_URL=/auth \
    KEYCLOAK_CLIENT_ID=securepatrol-web \
    KEYCLOAK_REALMS=securepatrol

COPY --from=builder /app/dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY docker-entrypoint.d/40-securepatrol-config.sh /docker-entrypoint.d/40-securepatrol-config.sh
# Strip CR in case the script was checked out with Windows line endings.
RUN sed -i 's/\r$//' /docker-entrypoint.d/40-securepatrol-config.sh \
 && chmod 755 /docker-entrypoint.d/40-securepatrol-config.sh

EXPOSE 80

CMD ["nginx", "-g", "daemon off;"]
