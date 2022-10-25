package tracer

type Config struct {
	EnableMemory     bool // enable memory capture
	DisableStack     bool // disable stack capture
	DisableStorage   bool // disable storage capture
	EnableReturnData bool // enable return data capture
	Limit            int  // maximum length of output, but zero means unlimited
}