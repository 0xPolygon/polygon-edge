package client

import (
	"encoding/json"

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

func (c *Client) get(uri string, out interface{}) error {
	req := fasthttp.AcquireRequest()
	res := fasthttp.AcquireResponse()

	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(res)

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
