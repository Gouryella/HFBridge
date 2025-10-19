# Build frontend
FROM node:22-alpine AS frontend-builder

WORKDIR /app/frontend
COPY frontend/package.json  ./
RUN corepack enable && pnpm install

COPY frontend/ ./
RUN pnpm build

# Build backend
FROM golang:1.24-alpine AS backend-builder
ARG BIN_NAME=hfbridge 
WORKDIR /app/backend
RUN apk add --no-cache build-base sqlite-dev
COPY backend/go.mod backend/go.sum ./
RUN go mod download

COPY backend/ ./
# Copy frontend build output to backend static directory expected by go:embed
RUN rm -rf ./static/web && mkdir -p ./static/web
COPY --from=frontend-builder /app/frontend/out ./static/web/

RUN CGO_ENABLED=1 GOOS=linux go build -o /app/hfbridge ./cmd/hfbridge/

# Final stage
FROM alpine:latest

WORKDIR /app
RUN apk add --no-cache ca-certificates sqlite-libs

COPY --from=backend-builder /app/hfbridge .

# Environment variables with defaults
ENV BIND_ADDR=:8080
ENV DEFAULT_UPSTREAM=https://huggingface.co
ENV DB_PATH=/data/hfbridge.db

# Create data directory
RUN mkdir -p /data

EXPOSE 8080

CMD ["./hfbridge"]
