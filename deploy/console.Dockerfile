FROM node:22-alpine AS deps

WORKDIR /app

COPY web/console/package.json web/console/package-lock.json ./
RUN npm ci

FROM node:22-alpine AS build

WORKDIR /app

ARG API_PROXY_TARGET=http://gateway:8080
ARG NEXT_PUBLIC_ENABLE_PASSWORDLESS_EMAIL=false
ARG NEXT_PUBLIC_ENABLE_PASSWORDLESS_MOCK=false
ENV API_PROXY_TARGET=$API_PROXY_TARGET
ENV NEXT_PUBLIC_ENABLE_PASSWORDLESS_EMAIL=$NEXT_PUBLIC_ENABLE_PASSWORDLESS_EMAIL
ENV NEXT_PUBLIC_ENABLE_PASSWORDLESS_MOCK=$NEXT_PUBLIC_ENABLE_PASSWORDLESS_MOCK

COPY --from=deps /app/node_modules ./node_modules
COPY web/console ./
RUN npm run build

FROM node:22-alpine

WORKDIR /app

ENV NODE_ENV=production
ENV PORT=3000
ENV HOSTNAME=0.0.0.0

COPY --from=build /app/.next/standalone ./
COPY --from=build /app/.next/static ./.next/static

EXPOSE 3000

CMD ["node", "server.js"]
