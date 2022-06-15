package peers

import "errors"

var (
	ErrorHeaderNotFound = errors.New("header not found")
	ErrorBlockNotFound  = errors.New("block not found")
)
