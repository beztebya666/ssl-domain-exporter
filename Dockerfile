# Stage 1: Build frontend
FROM node:20-alpine AS frontend-builder
WORKDIR /app/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

# Stage 2: Build backend
FROM golang:1.21-alpine AS backend-builder
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
ARG APP_VERSION=v1.3.0
ARG UI_VERSION=v1.3.0
ARG BUILD_TIME=unknown
ARG GIT_COMMIT=unknown
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w -X 'main.AppVersion=${APP_VERSION}' -X 'main.UIVersion=${UI_VERSION}' -X 'main.BuildTime=${BUILD_TIME}' -X 'main.GitCommit=${GIT_COMMIT}'" -o server ./cmd/server

# Stage 3: Final image
FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata wget \
 && addgroup -S app \
 && adduser -S -u 10001 -G app app
WORKDIR /app

COPY --from=backend-builder /app/server .
COPY --from=frontend-builder /app/frontend/dist ./frontend/dist

RUN mkdir -p /app/data \
 && chown -R app:app /app

EXPOSE 8080

VOLUME ["/app/data"]

ENV GIN_MODE=release \
    CONFIG_DIR=/app/data

HEALTHCHECK --interval=30s --timeout=10s --start-period=10s --retries=3 \
  CMD wget -qO- http://localhost:8080/health || exit 1

USER app

CMD ["./server"]
