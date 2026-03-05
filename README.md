# Telegram Service
****
## Запуск

```linux
 go mod tidy
```
```linux
 protoc --go_out=. --go-grpc_out=. proto/telegram.proto
```
```linux
 go run cmd/server/main.go
```

****

## Окружение
1. `APP_ID` - id приложения `*`
2. `APP_HASH` - hash приложения `*`
3. `SERVER_HOST` - ip/домен сервера
4. `SERVER_PORT` - порт сервера
5. `ENV` - окружение (prod/test/local)

`*` - обязательно к заполнению

****

## Примеры запросов

**Создание сессии: CreateSession**
```cmd
grpcurl -d '{}' -plaintext <SERVER_HOST>:<SERVER_PORT> telegram.TelegramService/CreateSession
```

**Удаление сессии: DeleteSession**

`session_od` - id полученное из `CreateSession`
```cmd
grpcurl -d '{ "session_id": "<session_id>" }' -plaintext <SERVER_HOST>:<SERVER_PORT> telegram.TelegramService/DeleteSession
```
