package jsonrpc

// Txpool is the txpool jsonrpc endpoint
type Txpool struct {
	d *Dispatcher
}

func (t *Txpool) Content() (interface{}, error) {
	return "hello", nil
}

func (t *Txpool) Inspect() (interface{}, error) {
	return "hello", nil
}

func (t *Txpool) Status() (interface{}, error) {
	return "hello", nil
}