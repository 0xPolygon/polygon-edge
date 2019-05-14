package http

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/pprof"

	"github.com/buaazp/fasthttprouter"
	"github.com/hashicorp/go-hclog"
	"github.com/umbracle/minimal/api"
	"github.com/umbracle/minimal/minimal"
	"github.com/valyala/fasthttp"
	"github.com/valyala/fasthttp/fasthttpadaptor"
)

const (
	defaultHTTPAddr = "127.0.0.1"
	defaultHTTPPort = 8600
)

// Factory is the factory method for the api backend
func Factory(logger hclog.Logger, m interface{}, config map[string]interface{}) (api.API, error) {
	h := &HTTP{
		m: m.(*minimal.Minimal),
	}

	var enableDebug bool
	debugRaw, ok := config["debug"]
	if ok {
		if enableDebug, ok = debugRaw.(bool); !ok {
			return nil, fmt.Errorf("could not convert debug flag to bool")
		}
	}

	tcpAddr, err := api.ReadAddrFromConfig(defaultHTTPAddr, defaultHTTPPort, config)
	if err != nil {
		return nil, err
	}

	router := fasthttprouter.New()
	router.GET("/v1/peers", h.wrap(h.PeersList))
	router.GET("/v1/peers/:peerid", h.wrap(h.PeersPeerID))

	if enableDebug {
		router.GET("/debug/pprof/", fastAdapt(pprof.Index))
		router.GET("/debug/pprof/cmdline", fastAdapt(pprof.Cmdline))
		router.GET("/debug/pprof/profile", fastAdapt(pprof.Profile))
		router.GET("/debug/pprof/symbol", fastAdapt(pprof.Symbol))
		router.GET("/debug/pprof/trace", fastAdapt(pprof.Trace))
	}

	lis, err := net.Listen("tcp", tcpAddr.String())
	if err != nil {
		return nil, err
	}

	h.http = &fasthttp.Server{
		Handler: router.Handler,
	}
	go func() {
		if err := h.http.Serve(lis); err != nil {
			logger.Info("Http closed: %v", err)
		}
	}()

	return h, nil
}

func fastAdapt(f func(w http.ResponseWriter, r *http.Request)) fasthttp.RequestHandler {
	return fasthttpadaptor.NewFastHTTPHandlerFunc(f)
}

// HTTP is an Api backend
type HTTP struct {
	m    *minimal.Minimal
	http *fasthttp.Server
}

func (h *HTTP) wrap(handler func(ctx *fasthttp.RequestCtx) (interface{}, error)) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		obj, err := handler(ctx)
		if err != nil {
			ctx.Error(err.Error(), http.StatusInternalServerError)
			return
		}
		if obj == nil {
			return
		}
		buf, err := json.Marshal(obj)
		if err != nil {
			ctx.Error(err.Error(), http.StatusInternalServerError)
			return
		}
		ctx.Response.Header.Set("Content-Type", "application/json")
		fmt.Fprintf(ctx, string(buf))
	}
}

// Close implements the Api interface
func (h *HTTP) Close() error {
	return h.http.Shutdown()
}
