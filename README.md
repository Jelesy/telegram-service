# Telegram Service

Сервис для подключения к телеграму на фреймворке gRPC
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

`session_id` - id полученное из `CreateSession`
```cmd
grpcurl -d '{ "session_id": "<session_id>" }' -plaintext <SERVER_HOST>:<SERVER_PORT> telegram.TelegramService/DeleteSession
```

**Отправка сообщения: SendMessage**

1. `peer` - имя пользователя, номер телефона или ссылка на профиль (*"@durov"*, *"+13115552368"*, *"t.me/telegram"* )
2. `text` - текст сообщения

```cmd
grpcurl -d '{ "peer": "<peer>",
"session_id": "<session_id>",
"text": "<text>" }' -plaintext <SERVER_HOST>:<SERVER_PORT> telegram.TelegramService/SendMessage
```

**Отправка фото: SendPhoto**

*Отправка фото по ссылке*

`photo` - url фото

```cmd
grpcurl -d '{ "peer": "<peer>",
"session_id": "<session_id>",
"photo": "<photo_url>" }' -plaintext <SERVER_HOST>:<SERVER_PORT> telegram.TelegramService/SendPhoto
```
