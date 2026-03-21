# Stage 1: Build frontend
FROM node:20-alpine AS frontend-builder
WORKDIR /app/frontend
COPY frontend/package.json ./
RUN npm install
COPY frontend/ ./
RUN npm run build

# Stage 2: Build backend
FROM golang:1.21-alpine AS backend-builder
RUN apk add --no-cache ca-certificates tzdata git
WORKDIR /app
COPY go.mod go.sum* ./
RUN go mod download || true
COPY . .
RUN go mod tidy && \
    CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o server ./cmd/server

# Stage 3: Final image
FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata wget
WORKDIR /app

COPY --from=backend-builder /app/server .
COPY --from=frontend-builder /app/frontend/dist ./frontend/dist
COPY config.yaml .

RUN mkdir -p /app/data

EXPOSE 8080

VOLUME ["/app/data"]

ENV GIN_MODE=release

HEALTHCHECK --interval=30s --timeout=10s --start-period=10s --retries=3 \
  CMD wget -qO- http://localhost:8080/health || exit 1

CMD ["./server"]
