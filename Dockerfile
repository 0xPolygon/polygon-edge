FROM golang:1.18.8-alpine as build_base

WORKDIR /src

COPY go.mod .
COPY go.sum .

RUN apk add --no-cache gcc librdkafka-dev libc-dev
RUN cd /src && go mod download

FROM build_base AS builder

ADD . /src
RUN cd /src && go build --tags musl -o polygon-edge main.go

FROM alpine:3.14

RUN set -x \
    && apk add --update --no-cache \
       ca-certificates \
    && rm -rf /var/cache/apk/*

COPY --from=builder /src/polygon-edge /usr/local/bin

EXPOSE 8545 9632 1478
ENTRYPOINT ["polygon-edge"]
