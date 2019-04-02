package jsonrpc

import (
	"io"
	"log"
	"net"
	"testing"
	"time"
)

func TestIPCPipe(t *testing.T) {

	startIPC()
	time.Sleep(5000 * time.Second)

	/*
		go server()
		time.Sleep(500 * time.Millisecond)
		go client()
		time.Sleep(500 * time.Millisecond)
	*/
}

func reader(r io.Reader) {
	buf := make([]byte, 1024)
	for {
		n, err := r.Read(buf[:])
		if err != nil {
			return
		}
		println("Client got:", string(buf[0:n]))
	}
}

func client() {
	c, err := net.Dial("unix", "/tmp/echo.sock")
	if err != nil {
		panic(err)
	}
	defer c.Close()

	go reader(c)
	for {
		_, err := c.Write([]byte("hi"))
		if err != nil {
			log.Fatal("write error:", err)
			break
		}
		time.Sleep(1e9)
	}
}

func echoServer(c net.Conn) {
	for {
		buf := make([]byte, 512)
		nr, err := c.Read(buf)
		if err != nil {
			return
		}

		data := buf[0:nr]
		println("Server got:", string(data))
		_, err = c.Write(data)
		if err != nil {
			log.Fatal("Write: ", err)
		}
	}
}

func server() {
	l, err := net.Listen("unix", "/tmp/echo.sock")
	if err != nil {
		log.Fatal("listen error:", err)
	}

	for {
		fd, err := l.Accept()
		if err != nil {
			log.Fatal("accept error:", err)
		}

		go echoServer(fd)
	}
}
