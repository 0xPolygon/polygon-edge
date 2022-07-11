package jsonrpc

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strings"
	"unicode"

	"github.com/hashicorp/go-hclog"
)

type serviceData struct {
	sv      reflect.Value
	funcMap map[string]*funcData
}

type funcData struct {
	inNum int
	reqt  []reflect.Type
	fv    reflect.Value
	isDyn bool
}

func (f *funcData) numParams() int {
	return f.inNum - 1
}

type endpoints struct {
	Eth    *Eth
	Web3   *Web3
	Net    *Net
	TxPool *TxPool
}

// Dispatcher handles all json rpc requests by delegating
// the execution flow to the corresponding service
type Dispatcher struct {
	logger        hclog.Logger
	serviceMap    map[string]*serviceData
	filterManager *FilterManager
	endpoints     endpoints
	chainID       uint64
	priceLimit    uint64
}

func newDispatcher(logger hclog.Logger, store JSONRPCStore, chainID uint64, priceLimit uint64) *Dispatcher {
	d := &Dispatcher{
		logger:     logger.Named("dispatcher"),
		chainID:    chainID,
		priceLimit: priceLimit,
	}

	if store != nil {
		d.filterManager = NewFilterManager(logger, store)
		go d.filterManager.Run()
	}

	d.registerEndpoints(store)

	return d
}

func (d *Dispatcher) registerEndpoints(store JSONRPCStore) {
	d.endpoints.Eth = &Eth{d.logger, store, d.chainID, d.filterManager, d.priceLimit}
	d.endpoints.Net = &Net{store, d.chainID}
	d.endpoints.Web3 = &Web3{}
	d.endpoints.TxPool = &TxPool{store}

	d.registerService("eth", d.endpoints.Eth)
	d.registerService("net", d.endpoints.Net)
	d.registerService("web3", d.endpoints.Web3)
	d.registerService("txpool", d.endpoints.TxPool)
}

func (d *Dispatcher) getFnHandler(req Request) (*serviceData, *funcData, Error) {
	callName := strings.SplitN(req.Method, "_", 2)
	if len(callName) != 2 {
		return nil, nil, NewMethodNotFoundError(req.Method)
	}

	serviceName, funcName := callName[0], callName[1]

	service, ok := d.serviceMap[serviceName]
	if !ok {
		return nil, nil, NewMethodNotFoundError(req.Method)
	}

	fd, ok := service.funcMap[funcName]

	if !ok {
		return nil, nil, NewMethodNotFoundError(req.Method)
	}

	return service, fd, nil
}

type wsConn interface {
	WriteMessage(messageType int, data []byte) error
}

// as per https://www.jsonrpc.org/specification, the `id` in JSON-RPC 2.0
// can only be a string or a non-decimal integer
func formatFilterResponse(id interface{}, resp string) (string, Error) {
	switch t := id.(type) {
	case string:
		return fmt.Sprintf(`{"jsonrpc":"2.0","id":"%s","result":"%s"}`, t, resp), nil
	case float64:
		if t == math.Trunc(t) {
			return fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"result":"%s"}`, int(t), resp), nil
		} else {
			return "", NewInvalidRequestError("Invalid json request")
		}
	case nil:
		return fmt.Sprintf(`{"jsonrpc":"2.0","id":null,"result":"%s"}`, resp), nil
	default:
		return "", NewInvalidRequestError("Invalid json request")
	}
}
func (d *Dispatcher) handleSubscribe(req Request, conn wsConn) (string, Error) {
	var params []interface{}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return "", NewInvalidRequestError("Invalid json request")
	}

	if len(params) == 0 {
		return "", NewInvalidParamsError("Invalid params")
	}

	subscribeMethod, ok := params[0].(string)
	if !ok {
		return "", NewSubscriptionNotFoundError(subscribeMethod)
	}

	var filterID string
	if subscribeMethod == "newHeads" {
		filterID = d.filterManager.NewBlockFilter(conn)
	} else if subscribeMethod == "logs" {
		logQuery, err := decodeLogQueryFromInterface(params[1])
		if err != nil {
			return "", NewInternalError(err.Error())
		}
		filterID = d.filterManager.NewLogFilter(logQuery, conn)
	} else {
		return "", NewSubscriptionNotFoundError(subscribeMethod)
	}

	return filterID, nil
}

func (d *Dispatcher) handleUnsubscribe(req Request) (bool, Error) {
	var params []interface{}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return false, NewInvalidRequestError("Invalid json request")
	}

	if len(params) != 1 {
		return false, NewInvalidParamsError("Invalid params")
	}

	filterID, ok := params[0].(string)
	if !ok {
		return false, NewSubscriptionNotFoundError(filterID)
	}

	return d.filterManager.Uninstall(filterID), nil
}

func (d *Dispatcher) HandleWs(reqBody []byte, conn wsConn) ([]byte, error) {
	var req Request
	if err := json.Unmarshal(reqBody, &req); err != nil {
		return NewRPCResponse(req.ID, "2.0", nil, NewInvalidRequestError("Invalid json request")).Bytes()
	}

	// if the request method is eth_subscribe we need to create a
	// new filter with ws connection
	if req.Method == "eth_subscribe" {
		filterID, err := d.handleSubscribe(req, conn)
		if err != nil {
			return NewRPCResponse(req.ID, "2.0", nil, err).Bytes()
		}

		resp, err := formatFilterResponse(req.ID, filterID)

		if err != nil {
			return NewRPCResponse(req.ID, "2.0", nil, err).Bytes()
		}

		return []byte(resp), nil
	}

	if req.Method == "eth_unsubscribe" {
		ok, err := d.handleUnsubscribe(req)
		if err != nil {
			return nil, err
		}

		res := "false"
		if ok {
			res = "true"
		}

		resp, err := formatFilterResponse(req.ID, res)
		if err != nil {
			return NewRPCResponse(req.ID, "2.0", nil, err).Bytes()
		}

		return []byte(resp), nil
	}

	// its a normal query that we handle with the dispatcher
	resp, err := d.handleReq(req)
	if err != nil {
		return nil, err
	}

	return NewRPCResponse(req.ID, "2.0", resp, err).Bytes()
}

func (d *Dispatcher) Handle(reqBody []byte) ([]byte, error) {
	x := bytes.TrimLeft(reqBody, " \t\r\n")
	if len(x) == 0 {
		return NewRPCResponse(nil, "2.0", nil, NewInvalidRequestError("Invalid json request")).Bytes()
	}

	if x[0] == '{' {
		var req Request
		if err := json.Unmarshal(reqBody, &req); err != nil {
			return NewRPCResponse(nil, "2.0", nil, NewInvalidRequestError("Invalid json request")).Bytes()
		}

		if req.Method == "" {
			return NewRPCResponse(req.ID, "2.0", nil, NewInvalidRequestError("Invalid json request")).Bytes()
		}

		resp, err := d.handleReq(req)

		return NewRPCResponse(req.ID, "2.0", resp, err).Bytes()
	}

	// handle batch requests
	var requests []Request
	if err := json.Unmarshal(reqBody, &requests); err != nil {
		return NewRPCResponse(nil, "2.0", nil, NewInvalidRequestError("Invalid json request")).Bytes()
	}

	responses := make([]Response, 0)

	for _, req := range requests {
		var response, err = d.handleReq(req)
		if err != nil {
			errorResponse := NewRPCResponse(req.ID, "2.0", nil, err)
			responses = append(responses, errorResponse)

			continue
		}

		resp := NewRPCResponse(req.ID, "2.0", response, nil)
		responses = append(responses, resp)
	}

	respBytes, err := json.Marshal(responses)
	if err != nil {
		return NewRPCResponse(nil, "2.0", nil, NewInternalError("Internal error")).Bytes()
	}

	return respBytes, nil
}

func (d *Dispatcher) handleReq(req Request) ([]byte, Error) {
	d.logger.Debug("request", "method", req.Method, "id", req.ID)

	service, fd, ferr := d.getFnHandler(req)
	if ferr != nil {
		return nil, ferr
	}

	inArgs := make([]reflect.Value, fd.inNum)
	inArgs[0] = service.sv

	inputs := make([]interface{}, fd.numParams())

	for i := 0; i < fd.inNum-1; i++ {
		val := reflect.New(fd.reqt[i+1])
		inputs[i] = val.Interface()
		inArgs[i+1] = val.Elem()
	}

	if fd.numParams() > 0 {
		if err := json.Unmarshal(req.Params, &inputs); err != nil {
			return nil, NewInvalidParamsError("Invalid Params")
		}
	}

	output := fd.fv.Call(inArgs)
	if err := getError(output[1]); err != nil {
		d.logInternalError(req.Method, err)

		return nil, NewInvalidRequestError(err.Error())
	}

	var (
		data []byte
		err  error
	)

	if res := output[0].Interface(); res != nil {
		data, err = json.Marshal(res)
		if err != nil {
			d.logInternalError(req.Method, err)

			return nil, NewInternalError("Internal error")
		}
	}

	return data, nil
}

func (d *Dispatcher) logInternalError(method string, err error) {
	d.logger.Error("failed to dispatch", "method", method, "err", err)
}

func (d *Dispatcher) registerService(serviceName string, service interface{}) {
	if d.serviceMap == nil {
		d.serviceMap = map[string]*serviceData{}
	}

	if serviceName == "" {
		panic("jsonrpc: serviceName cannot be empty")
	}

	st := reflect.TypeOf(service)
	if st.Kind() == reflect.Struct {
		panic(fmt.Sprintf("jsonrpc: service '%s' must be a pointer to struct", serviceName))
	}

	funcMap := make(map[string]*funcData)

	for i := 0; i < st.NumMethod(); i++ {
		mv := st.Method(i)
		if mv.PkgPath != "" {
			// skip unexported methods
			continue
		}

		name := lowerCaseFirst(mv.Name)
		funcName := serviceName + "_" + name
		fd := &funcData{
			fv: mv.Func,
		}

		var err error

		if fd.inNum, fd.reqt, err = validateFunc(funcName, fd.fv, true); err != nil {
			panic(fmt.Sprintf("jsonrpc: %s", err))
		}
		// check if last item is a pointer
		if fd.numParams() != 0 {
			last := fd.reqt[fd.numParams()]
			if last.Kind() == reflect.Ptr {
				fd.isDyn = true
			}
		}

		funcMap[name] = fd
	}

	d.serviceMap[serviceName] = &serviceData{
		sv:      reflect.ValueOf(service),
		funcMap: funcMap,
	}
}

func validateFunc(funcName string, fv reflect.Value, _ bool) (inNum int, reqt []reflect.Type, err error) {
	if funcName == "" {
		err = fmt.Errorf("funcName cannot be empty")

		return
	}

	ft := fv.Type()
	if ft.Kind() != reflect.Func {
		err = fmt.Errorf("function '%s' must be a function instead of %s", funcName, ft)

		return
	}

	inNum = ft.NumIn()

	if outNum := ft.NumOut(); ft.NumOut() != 2 {
		err = fmt.Errorf("unexpected number of output arguments in the function '%s': %d. Expected 2", funcName, outNum)

		return
	}

	if !isErrorType(ft.Out(1)) {
		err = fmt.Errorf(
			"unexpected type for the second return value of the function '%s': '%s'. Expected '%s'",
			funcName,
			ft.Out(1),
			errt,
		)

		return
	}

	reqt = make([]reflect.Type, inNum)
	for i := 0; i < inNum; i++ {
		reqt[i] = ft.In(i)
	}

	return
}

var errt = reflect.TypeOf((*error)(nil)).Elem()

func isErrorType(t reflect.Type) bool {
	return t.Implements(errt)
}

func getError(v reflect.Value) error {
	if v.IsNil() {
		return nil
	}

	extractedErr, ok := v.Interface().(error)
	if !ok {
		return errors.New("invalid type assertion, unable to extract error")
	}

	return extractedErr
}

func lowerCaseFirst(str string) string {
	for i, v := range str {
		return string(unicode.ToLower(v)) + str[i+1:]
	}

	return ""
}
