fuser -k 8081/tcp
go mod tidy
set -a && source .env && set +a
go test ./...
go run .