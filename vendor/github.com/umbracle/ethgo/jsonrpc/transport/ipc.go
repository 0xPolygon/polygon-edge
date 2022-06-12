package transport

import (
	"encoding/json"
	"net"
)

func newIPC(addr string) (Transport, error) {
	conn, err := net.Dial("unix", addr)
	if err != nil {
		return nil, err
	}

	codec := &ipcCodec{
		buf:  json.RawMessage{},
		conn: conn,
		dec:  json.NewDecoder(conn),
	}

	return newStream(codec)
}

type ipcCodec struct {
	buf  json.RawMessage
	conn net.Conn
	dec  *json.Decoder
}

func (i *ipcCodec) Close() error {
	return i.conn.Close()
}

func (i *ipcCodec) Read(b []byte) ([]byte, error) {
	i.buf = i.buf[:0]
	if err := i.dec.Decode(&i.buf); err != nil {
		return nil, err
	}
	b = append(b, i.buf...)
	return b, nil
}

func (i *ipcCodec) Write(b []byte) error {
	_, err := i.conn.Write(b)
	return err
}
