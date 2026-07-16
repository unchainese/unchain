# 构建阶段
FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o app .

# 运行阶段
FROM scratch
WORKDIR /app
COPY --from=builder /app/app .


ENV APP_ENV=production
ENV APP_PORT=80
ENV REGISTER_URL=https://unchainapi.bob99.workers.dev/api/node
ENV APP_HOST=a.mojocn.com
ENV ALLOW_USERS=903bcd04-79e7-429c-bf0c-0456c7de9cdc,903bcd04-79e7-429c-bf0c-0456c7de9cd1
ENV INTERVAL_SECOND=3600
ENV ENABLE_DATA_USAGE_METERING=true

EXPOSE 80
ENTRYPOINT ["./app", "run"]