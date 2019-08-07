package client

import (
	"encoding/json"
	"fmt"

	"github.com/valyala/fasthttp"
)

// Client is an http client for the http api backend
type Client struct {
	addr   string
	client *fasthttp.Client
}

// NewClient creates a new instance of the client
func NewClient(addr string) *Client {
	return &Client{
		addr:   addr,
		client: &fasthttp.Client{},
	}
}

func (c *Client) post(uri string, input map[string]string) error {
	req, res := fasthttp.AcquireRequest(), fasthttp.AcquireResponse()

	defer func() {
		fasthttp.ReleaseRequest(req)
		fasthttp.ReleaseResponse(res)
	}()

	args := req.PostArgs()
	for k, v := range input {
		args.Set(k, v)
	}

	req.SetRequestURI(c.addr + uri)
	req.Header.SetMethod("POST")

	if err := c.client.Do(req, res); err != nil {
		return err
	}
	if len(res.Body()) != 0 {
		return fmt.Errorf(string(res.Body()))
	}
	return nil
}

func (c *Client) get(uri string, out interface{}) error {
	req, res := fasthttp.AcquireRequest(), fasthttp.AcquireResponse()

	defer func() {
		fasthttp.ReleaseRequest(req)
		fasthttp.ReleaseResponse(res)
	}()

	req.SetRequestURI(c.addr + uri)
	req.Header.SetMethod("GET")

	err := c.client.Do(req, res)
	if err != nil {
		return err
	}

	if err := json.Unmarshal(res.Body(), out); err != nil {
		return err
	}
	return nil
}

// PeersList returns a list of peers
func (c *Client) PeersList() ([]string, error) {
	var out []string
	err := c.get("/v1/peers", &out)
	return out, err
}

// PeersInfo returns specific info about one peer
func (c *Client) PeersInfo(peerid string) (map[string]interface{}, error) {
	var out map[string]interface{}
	err := c.get("/v1/peers/"+peerid, &out)
	return out, err
}

// PeersAdd adds a new peer
func (c *Client) PeersAdd(peerid string) error {
	return c.post("/v1/peers", map[string]string{
		"peer": peerid,
	})
}
