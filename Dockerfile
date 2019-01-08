FROM golang:alpine AS builder

RUN apk add --no-cache --update git

RUN go get -v -u github.com/AlexStocks/supervisord

RUN CGO_ENABLED=0 GOOS=linux go build -a -ldflags "-extldflags -static" -o /usr/local/bin/supervisord github.com/AlexStocks/supervisord

FROM scratch

COPY --from=builder /usr/local/bin/supervisord /usr/local/bin/supervisord

ENTRYPOINT ["/usr/local/bin/supervisord"]
