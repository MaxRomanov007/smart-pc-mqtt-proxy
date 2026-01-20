# Stage 1: Build
FROM golang:1.25.1-alpine AS build

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o server ./cmd/mqtt-proxy/main.go

# Stage 2: Runtime
FROM alpine:3.20

WORKDIR /app

COPY --from=build /app/server .
EXPOSE 8080

CMD ["./server"]