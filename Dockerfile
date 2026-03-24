# Pre-built binaries are copied from the local machine (built with GOOS=linux GOARCH=amd64).
# Run before docker compose: GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o server ./cmd/api
#                             GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o worker ./cmd/worker
FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY server .
COPY worker .
EXPOSE 8080
CMD ["./server"]
