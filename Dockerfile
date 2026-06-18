FROM golang:1.25.5-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/skuld-cli .

FROM alpine:3.22 AS runtime

RUN apk add --no-cache ca-certificates git && \
    adduser -D -h /home/skuld-daemon skuld-daemon && \
    install -d -m 0700 -o skuld-daemon -g skuld-daemon /home/skuld-daemon/.config/skuld-cli/skuldd

COPY --from=builder /out/skuld-cli /usr/local/bin/skuld-cli

USER skuld-daemon
WORKDIR /home/skuld-daemon

ENTRYPOINT ["/usr/local/bin/skuld-cli"]
CMD ["--help"]
