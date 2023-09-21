FROM golang:1.20-alpine AS go-builder

WORKDIR /polygon-edge

COPY go.mod ./go.mod 
COPY go.sum ./go.sum
RUN go mod download

COPY . .

RUN go build -o polygon-edge main.go

FROM node:18.18.0-alpine3.18 AS node-builder

WORKDIR /polygon-edge

COPY ./core-contracts ./core-contracts

WORKDIR /polygon-edge/core-contracts

RUN npm install
RUN npx hardhat compile

FROM alpine:latest AS runner

RUN apk --no-cache add ca-certificates jq

WORKDIR /polygon-edge

COPY --from=go-builder /polygon-edge/polygon-edge ./
COPY ./docker/local/polygon-edge.sh ./
COPY --from=node-builder /polygon-edge/core-contracts/artifacts ./core-contracts/artifacts

# Expose json-rpc, libp2p and grpc ports
EXPOSE 8545 9632 1478 5001

ENTRYPOINT ["./polygon-edge.sh"]
