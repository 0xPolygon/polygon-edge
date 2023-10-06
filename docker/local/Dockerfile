FROM golang:1.20-alpine AS builder

RUN apk add make git

WORKDIR /polygon-edge

ADD go.mod go.sum ./
RUN go mod download

COPY . .

RUN make build

FROM alpine:latest AS runner

RUN apk --no-cache add ca-certificates jq

WORKDIR /polygon-edge

COPY --from=builder /polygon-edge/polygon-edge ./
COPY ./docker/local/polygon-edge.sh ./

# Expose json-rpc, libp2p and grpc ports
EXPOSE 8545 9632 1478 5001

ENTRYPOINT ["./polygon-edge.sh"]
