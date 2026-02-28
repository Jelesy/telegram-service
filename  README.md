# Telegram Service

## Запуск
1. go mod tidy
2. protoc --go_out=. --go-grpc_out=. proto/telegram.proto
3. go run cmd/server/main.go