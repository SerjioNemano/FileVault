# Используем Go 1.25
FROM golang:1.25


# Устанавливаем air для авто-перезапуска
RUN go install github.com/air-verse/air@v1.67.1

WORKDIR /app

# CMD запускаем air
CMD ["air"]