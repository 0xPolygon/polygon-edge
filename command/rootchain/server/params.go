package server

const (
	dataDirFlag = "data-dir"
	noConsole   = "no-console"
	premineFlag = "premine"
)

type serverParams struct {
	dataDir   string
	premine   []string
	noConsole bool
}
