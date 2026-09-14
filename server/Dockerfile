# Многоступенчатый Dockerfile для api / worker / relay.
# Таргеты: build (binary), api (API-сервер), worker (индексатор), relay (медиа-релей).

FROM golang:1.25-alpine AS build
WORKDIR /src

# go.mod может отсутствовать в ранних версиях, копируем каталог целиком.
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/api ./cmd/api && \
    CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/worker ./cmd/worker && \
    CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/relay ./cmd/relay

FROM alpine:3.20 AS runtime
RUN apk add --no-cache ca-certificates tzdata
COPY --from=build /out/api /usr/local/bin/api
COPY --from=build /out/worker /usr/local/bin/worker
COPY --from=build /out/relay /usr/local/bin/relay
COPY migrations /data/migrations
RUN mkdir -p /data/media_cache
VOLUME ["/data"]
WORKDIR /data
CMD ["sh"]