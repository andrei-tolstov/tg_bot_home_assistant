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

# Сборка статического бинарного файла без CGO
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /app/bot .

# Финальный легковесный образ
FROM alpine:latest

# Установка корневых SSL-сертификатов для HTTPS-запросов и данных таймзон
RUN apk --no-cache add ca-certificates tzdata && \
    adduser -D -g '' -u 10001 botuser

WORKDIR /app

# Копирование скомпилированного бинарного файла из этапа сборки
COPY --from=builder /app/bot /app/bot

# Запуск от имени непривилегированного пользователя
USER botuser

ENTRYPOINT ["/app/bot"]
