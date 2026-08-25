# Этап сборки (builder)
FROM golang:alpine AS builder

WORKDIR /app

# Установка необходимых утилит сборки и сертификатов
RUN apk add --no-cache git ca-certificates tzdata

# Кэширование зависимостей Go
COPY go.mod go.sum ./
RUN go mod download

# Копирование исходного кода
COPY . .

# Сборка статического бинарного файла без CGO в /usr/local/bin/tg-bot (чтобы не конфликтовать с директорией /app/bot)
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /usr/local/bin/tg-bot .

# Финальный легковесный образ
FROM alpine:latest

# Установка корневых SSL-сертификатов для HTTPS-запросов и данных таймзон
RUN apk --no-cache add ca-certificates tzdata && \
    adduser -D -g '' -u 10001 botuser

WORKDIR /app

# Копирование скомпилированного бинарного файла из этапа сборки
COPY --from=builder /usr/local/bin/tg-bot /usr/local/bin/tg-bot

# Запуск от имени непривилегированного пользователя
USER botuser

ENTRYPOINT ["/usr/local/bin/tg-bot"]
