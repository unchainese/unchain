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
ENV APP_HOST=svr.libragen.unchain
ENV APP_PORT=8880
ENV REGISTER_URL=https://unchain.libragen.cn/api/node
ENV REGISTER_TOKEN="unchain.people.from.censorship.and.surveillance"
ENV ALLOW_USERS=903bcd04-79e7-429c-bf0c-0456c7de9cdc,903bcd04-79e7-429c-bf0c-0456c7de9cd1
ENV LOG_FILE=
ENV DEBUG_LEVEL=DEBUG
ENV INTERVAL_SECOND=3600
ENV ENABLE_METERING=true
ENV BUFFER_SIZE=8192

EXPOSE 8880
ENTRYPOINT ["./app", "run"]