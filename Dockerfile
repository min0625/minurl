FROM golang:1.26.2 AS builder

WORKDIR /app

COPY go.mod ./
COPY go.sum ./
RUN go mod download

COPY . .

ARG LDFLAGS="-s -w"
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="${LDFLAGS}" -o minurl ./cmd/minurl

FROM gcr.io/distroless/static-debian12

COPY --from=builder /app/minurl /minurl

ENV MINURL_STORAGE_PATH=/data/minurl.sqlite3

VOLUME ["/data"]
EXPOSE 8888

ENTRYPOINT ["/minurl"]
