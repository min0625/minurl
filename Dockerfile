FROM golang:1.26.2 AS builder

WORKDIR /app

COPY go.mod ./
COPY go.sum ./
RUN go mod download

COPY . .

ARG LDFLAGS="-s -w"
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="${LDFLAGS}" -o hello-go ./cmd/minurl

FROM gcr.io/distroless/static-debian12

COPY --from=builder /app/hello-go /hello-go

ENTRYPOINT ["/hello-go"]
