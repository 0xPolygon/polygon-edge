
.PHONY:
build-rlpgen-tests:
	go run rlpgen/*.go --path ./rlpgen/tests --package types --objs Test1,Header,Transaction,Body,Block,Receipt,Log --include ./rlpgen/tests/external/external.go --output ./rlpgen/tests/types_encoding.go
