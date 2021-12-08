package jsonrpc

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"reflect"
	"strings"
	"unicode"

	"github.com/0xPolygon/polygon-sdk/types"
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
	Txpool *Txpool
}

// Dispatcher handles jsonrpc requests
type Dispatcher struct {
	logger        hclog.Logger
	store         blockchainInterface
	serviceMap    map[string]*serviceData
	endpoints     endpoints
	filterManager *FilterManager
	chainID       uint64
}

// newTestDispatcher returns a dispatcher without the filter manager, used for testing
func newTestDispatcher(logger hclog.Logger, store blockchainInterface) *Dispatcher {
	d := &Dispatcher{
		logger: logger.Named("dispatcher"),
		store:  store,
	}

	d.registerEndpoints()

	return d
}

func newDispatcher(logger hclog.Logger, store blockchainInterface, chainID uint64) *Dispatcher {
	d := &Dispatcher{
		logger:  logger.Named("dispatcher"),
		store:   store,
		chainID: chainID,
	}
	d.registerEndpoints()
	if store != nil {
		d.filterManager = NewFilterManager(logger, store)
		go d.filterManager.Run()
	}
	return d
}

func (d *Dispatcher) registerEndpoints() {
	d.endpoints.Eth = &Eth{d}
	d.endpoints.Net = &Net{d}
	d.endpoints.Web3 = &Web3{d}
	d.endpoints.Txpool = &Txpool{d}

	d.registerService("eth", d.endpoints.Eth)
	d.registerService("net", d.endpoints.Net)
	d.registerService("web3", d.endpoints.Web3)
	d.registerService("txpool", d.endpoints.Txpool)
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
		return "", NewSubscriptionNotFoundError(params[0].(string))
	}

	var filterID string
	if subscribeMethod == "newHeads" {
		filterID = d.filterManager.NewBlockFilter(conn)

	} else if subscribeMethod == "logs" {
		logFilter, err := decodeLogFilterFromInterface(params[1])
		if err != nil {
			return "", NewInternalError(err.Error())
		}
		filterID = d.filterManager.NewLogFilter(logFilter, conn)

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
		return false, NewSubscriptionNotFoundError(params[0].(string))
	}

	return d.filterManager.Uninstall(filterID), nil
}

func (d *Dispatcher) HandleWs(reqBody []byte, conn wsConn) ([]byte, error) {
	var req Request
	if err := json.Unmarshal(reqBody, &req); err != nil {

		return NewRpcResponse(req.ID, "2.0", nil, NewInvalidRequestError("Invalid json request")).Bytes()
	}

	// if the request method is eth_subscribe we need to create a
	// new filter with ws connection
	if req.Method == "eth_subscribe" {
		filterID, err := d.handleSubscribe(req, conn)
		if err != nil {
			NewRpcResponse(req.ID, "2.0", nil, err).Bytes()
		}
		resp, err := formatFilterResponse(req.ID, filterID)
		if err != nil {
			return NewRpcResponse(req.ID, "2.0", nil, err).Bytes()
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
			return NewRpcResponse(req.ID, "2.0", nil, err).Bytes()
		}

		return []byte(resp), nil

	}

	// its a normal query that we handle with the dispatcher
	resp, err := d.handleReq(req)
	if err != nil {
		return nil, err
	}
	return NewRpcResponse(req.ID, "2.0", resp, err).Bytes()
}

func (d *Dispatcher) Handle(reqBody []byte) ([]byte, error) {

	x := bytes.TrimLeft(reqBody, " \t\r\n")
	if len(x) == 0 {
		return NewRpcResponse(nil, "2.0", nil, NewInvalidRequestError("Invalid json request")).Bytes()
	}
	if x[0] == '{' {
		var req Request
		if err := json.Unmarshal(reqBody, &req); err != nil {

			return NewRpcResponse(nil, "2.0", nil, NewInvalidRequestError("Invalid json request")).Bytes()
		}
		if req.Method == "" {
			return NewRpcResponse(req.ID, "2.0", nil, NewInvalidRequestError("Invalid json request")).Bytes()
		}

		resp, err := d.handleReq(req)

		return NewRpcResponse(req.ID, "2.0", resp, err).Bytes()
	}

	// handle batch requests
	var requests []Request
	if err := json.Unmarshal(reqBody, &requests); err != nil {
		return NewRpcResponse(nil, "2.0", nil, NewInvalidRequestError("Invalid json request")).Bytes()
	}
	var responses []Response
	for _, req := range requests {
		var response, err = d.handleReq(req)
		if err != nil {
			errorResponse := NewRpcResponse(req.ID, "2.0", nil, err)
			responses = append(responses, errorResponse)
			continue
		}

		resp := NewRpcResponse(req.ID, "2.0", response, nil)
		responses = append(responses, resp)
	}

	respBytes, err := json.Marshal(responses)
	if err != nil {
		return NewRpcResponse(nil, "2.0", nil, NewInternalError("Internal error")).Bytes()
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

	var data []byte
	var err error
	res := output[0].Interface()
	if res != nil {
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

func validateFunc(funcName string, fv reflect.Value, isMethod bool) (inNum int, reqt []reflect.Type, err error) {
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
	outNum := ft.NumOut()

	if outNum != 2 {
		err = fmt.Errorf("unexpected number of output arguments in the function '%s': %d. Expected 2", funcName, outNum)
		return
	}
	if !isErrorType(ft.Out(1)) {
		err = fmt.Errorf("unexpected type for the second return value of the function '%s': '%s'. Expected '%s'", funcName, ft.Out(1), errt)
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
	return v.Interface().(error)
}

func lowerCaseFirst(str string) string {
	for i, v := range str {
		return string(unicode.ToLower(v)) + str[i+1:]
	}
	return ""
}

func (d *Dispatcher) getBlockHeaderImpl(number BlockNumber) (*types.Header, error) {
	switch number {
	case LatestBlockNumber:
		return d.store.Header(), nil

	case EarliestBlockNumber:
		return nil, fmt.Errorf("fetching the earliest header is not supported")

	case PendingBlockNumber:
		return nil, fmt.Errorf("fetching the pending header is not supported")

	default:
		// Convert the block number from hex to uint64
		header, ok := d.store.GetHeaderByNumber(uint64(number))
		if !ok {
			return nil, fmt.Errorf("Error fetching block number %d header", uint64(number))
		}
		return header, nil
	}
}

// getNextNonce returns the next nonce for the account for the specified block
func (d *Dispatcher) getNextNonce(address types.Address, number BlockNumber) (uint64, error) {
	if number == PendingBlockNumber {
		// Grab the latest pending nonce from the TxPool
		//
		// If the account is not initialized in the local TxPool,
		// return the latest nonce from the world state
		res := d.store.GetNonce(address)

		return res, nil
	}

	header, err := d.getBlockHeaderImpl(number)
	if err != nil {
		return 0, err
	}
	acc, err := d.store.GetAccount(header.StateRoot, address)
	if errors.As(err, &ErrStateNotFound) {
		// If the account doesn't exist / isn't initialized,
		// return a nonce value of 0
		return 0, nil
	} else if err != nil {
		return 0, err
	}

	return acc.Nonce, nil
}

func (d *Dispatcher) decodeTxn(arg *txnArgs) (*types.Transaction, error) {
	if arg.Data != nil && arg.Input != nil {
		return nil, fmt.Errorf("both input and data cannot be set")
	}

	// set default values
	if arg.From == nil {
		arg.From = &types.ZeroAddress
		arg.Nonce = argUintPtr(0)
	} else if arg.Nonce == nil {
		// get nonce from the pool
		nonce, err := d.getNextNonce(*arg.From, LatestBlockNumber)
		if err != nil {
			return nil, err
		}
		arg.Nonce = argUintPtr(nonce)
	}
	if arg.Value == nil {
		arg.Value = argBytesPtr([]byte{})
	}
	if arg.GasPrice == nil {
		arg.GasPrice = argBytesPtr([]byte{})
	}

	var input []byte
	if arg.Data != nil {
		input = *arg.Data
	} else if arg.Input != nil {
		input = *arg.Input
	}
	if arg.To == nil {
		if input == nil {
			return nil, fmt.Errorf("contract creation without data provided")
		}
	}
	if input == nil {
		input = []byte{}
	}

	if arg.Gas == nil {
		arg.Gas = argUintPtr(0)
	}

	txn := &types.Transaction{
		From:     *arg.From,
		Gas:      uint64(*arg.Gas),
		GasPrice: new(big.Int).SetBytes(*arg.GasPrice),
		Value:    new(big.Int).SetBytes(*arg.Value),
		Input:    input,
		Nonce:    uint64(*arg.Nonce),
	}
	if arg.To != nil {
		txn.To = arg.To
	}
	txn.ComputeHash()
	return txn, nil
}
