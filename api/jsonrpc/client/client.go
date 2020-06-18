package client

import (
	"encoding/json"

	"github.com/0xPolygon/minimal/api/jsonrpc"
	"github.com/valyala/fasthttp"
)

// Client is an Ethereum JsonRPC client
type Client struct {
	addr   string
	client *fasthttp.Client
}

// NewClient creates a new Ethereum Jsonrpc client
func NewClient(addr string) *Client {
	return &Client{
		addr:   addr,
		client: &fasthttp.Client{},
	}
}

func (c *Client) do(method string, out interface{}, params ...interface{}) error {

	// Encode json-rpc request
	request := jsonrpc.Request{
		Method: method,
	}
	if len(params) > 0 {
		data, err := json.Marshal(params)
		if err != nil {
			panic(err)
		}
		request.Params = data
	}
	raw, err := json.Marshal(request)
	if err != nil {
		return err
	}

	req := fasthttp.AcquireRequest()
	res := fasthttp.AcquireResponse()

	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(res)

	req.SetRequestURI(c.addr)
	req.Header.SetMethod("POST")
	req.SetBody(raw)

	if err := c.client.Do(req, res); err != nil {
		return err
	}

	// Decode json-rpc response
	var response jsonrpc.Response
	if err := json.Unmarshal(res.Body(), &response); err != nil {
		return err
	}
	if response.Error != nil {
		return response.Error
	}
	if err := json.Unmarshal(response.Result, out); err != nil {
		return err
	}
	return nil
}
