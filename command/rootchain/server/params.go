package server

const (
	dataDirFlag = "data-dir"
	noConsole   = "no-console"
)

type serverParams struct {
	dataDir   string
	noConsole bool
}
