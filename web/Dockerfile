FROM node:24-alpine AS build
WORKDIR /app
# Contexto é a raiz do repositório: o build do web lê api/ (contrato e fixtures compartilhados com o Go).
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
COPY api/ /api/
RUN npm run build

FROM nginx:1.29-alpine
COPY web/nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /app/dist /usr/share/nginx/html
