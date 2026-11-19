FROM golang:1.22-alpine AS build

WORKDIR /src
RUN apk add --no-cache ca-certificates git
COPY go.mod go.sum ./
RUN go mod download
COPY main.go ./
COPY internal ./internal
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/auto-ticket-system .

FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S app \
    && adduser -S -D -H -u 10001 -G app app \
    && mkdir -p /app/static /data \
    && chown -R app:app /app /data
WORKDIR /app
COPY --from=build --chown=app:app /out/auto-ticket-system /app/auto-ticket-system
COPY --chown=app:app static/ /app/static/
ENV PORT=8080 \
    EXCEL_PATH=/data/employees.xlsx \
    APP_TIMEZONE=Asia/Dhaka \
    GIN_MODE=release
USER app
EXPOSE 8080
ENTRYPOINT ["/app/auto-ticket-system"]
