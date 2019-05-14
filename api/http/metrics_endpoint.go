package http

import (
	"net/http"
	"sync"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/valyala/fasthttp"
	"github.com/valyala/fasthttp/fasthttpadaptor"
)

var (
	promHandler http.Handler
	promOnce    sync.Once
)

// Metrics returns several client metrics
func (h *HTTP) Metrics(ctx *fasthttp.RequestCtx) (interface{}, error) {
	if string(ctx.QueryArgs().Peek("format")) == "prometheus" {
		handler := fasthttpadaptor.NewFastHTTPHandler(promhttp.Handler())
		handler(ctx)

		return nil, nil
	}

	var res interface{}
	var err error

	fasthttpadaptor.NewFastHTTPHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		res, err = h.m.InmemSink.DisplayMetrics(w, r)
	})(ctx)
	return res, err
}
