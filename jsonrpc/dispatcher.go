package jsonrpc

import (
	"encoding/json"
	"fmt"
	"math/big"
	"reflect"
	"strings"
	"unicode"

	"github.com/0xPolygon/minimal/types"
	"github.com/hashicorp/go-hclog"
)

var (
	invalidJSONRequest = &ErrorObject{Code: -32600, Message: "invalid json request"}
	internalError      = &ErrorObject{Code: -32603, Message: "internal error"}
)

func invalidMethod(method string) error {
	return &ErrorObject{Code: -32601, Message: fmt.Sprintf("The method %s does not exist/is not available", method)}
}

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
	Eth  *Eth
	Web3 *Web3
	Net  *Net
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

	d.registerService("eth", d.endpoints.Eth)
	d.registerService("net", d.endpoints.Net)
	d.registerService("web3", d.endpoints.Web3)
}

func (d *Dispatcher) getFnHandler(req Request) (*serviceData, *funcData, error) {
	callName := strings.SplitN(req.Method, "_", 2)
	if len(callName) != 2 {
		return nil, nil, invalidMethod(req.Method)
	}

	serviceName, funcName := callName[0], callName[1]

	service, ok := d.serviceMap[serviceName]
	if !ok {
		return nil, nil, invalidMethod(req.Method)
	}
	fd, ok := service.funcMap[funcName]
	if !ok {
		return nil, nil, invalidMethod(req.Method)
	}
	return service, fd, nil
}

type wsConn interface {
	WriteMessage(b []byte) error
}

func (d *Dispatcher) handleSubscribe(req Request, conn wsConn) (string, error) {
	var params []interface{}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return "", invalidJSONRequest
	}
	if len(params) == 0 {
		return "", invalidJSONRequest
	}

	subscribeMethod, ok := params[0].(string)
	if !ok {
		return "", fmt.Errorf("subscribe method '%s' not found", params[0])
	}

	var filterID string
	if subscribeMethod == "newHeads" {
		filterID = d.filterManager.NewBlockFilter(conn)

	} else if subscribeMethod == "logs" {
		logFilter, err := decodeLogFilterFromInterface(params[1])
		if err != nil {
			return "", err
		}
		filterID = d.filterManager.NewLogFilter(logFilter, conn)

	} else {
		return "", fmt.Errorf("subscribe method %s not found", subscribeMethod)
	}

	return filterID, nil
}

func (d *Dispatcher) handleUnsubscribe(req Request) (bool, error) {
	var params []interface{}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return false, invalidJSONRequest
	}
	if len(params) != 1 {
		return false, invalidJSONRequest
	}

	filterID, ok := params[0].(string)
	if !ok {
		return false, fmt.Errorf("unsubscribe filter not found")
	}

	return d.filterManager.Uninstall(filterID), nil
}

func (d *Dispatcher) HandleWs(reqBody []byte, conn wsConn) ([]byte, error) {
	var req Request
	if err := json.Unmarshal(reqBody, &req); err != nil {
		return nil, invalidJSONRequest
	}

	// if the request method is eth_subscribe we need to create a
	// new filter with ws connection
	if req.Method == "eth_subscribe" {
		filterID, err := d.handleSubscribe(req, conn)
		if err != nil {
			return nil, err
		}

		resp := fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"result":"%s"}`, req.ID, filterID)
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
		resp := fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"result":"%s"}`, req.ID, res)
		return []byte(resp), nil
	}

	// its a normal query that we handle with the dispatcher
	resp, err := d.handleReq(req)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func (d *Dispatcher) Handle(reqBody []byte) ([]byte, error) {
	var req Request
	if err := json.Unmarshal(reqBody, &req); err != nil {
		return nil, invalidJSONRequest
	}
	return d.handleReq(req)
}

func (d *Dispatcher) handleReq(req Request) ([]byte, error) {
	d.logger.Debug("request", "method", req.Method, "id", req.ID)

	service, fd, err := d.getFnHandler(req)
	if err != nil {
		return nil, err
	}

	inArgs := make([]reflect.Value, fd.inNum)
	inArgs[0] = service.sv

	inputs := make([]interface{}, fd.numParams())
	for i := 0; i < fd.inNum-1; i++ {
		val := reflect.New(fd.reqt[i+1])
		inputs[i] = val.Interface()
		inArgs[i+1] = val.Elem()
	}

	if err := json.Unmarshal(req.Params, &inputs); err != nil {
		return nil, invalidJSONRequest
	}

	output := fd.fv.Call(inArgs)
	err = getError(output[1])
	if err != nil {
		return nil, d.internalError(req.Method, err)
	}

	var data []byte
	res := output[0].Interface()
	if res != nil {
		data, err = json.Marshal(res)
		if err != nil {
			return nil, d.internalError(req.Method, err)
		}
	}

	resp := Response{
		ID:      req.ID,
		JSONRPC: "2.0",
		Result:  data,
	}
	respBytes, err := json.Marshal(resp)
	if err != nil {
		return nil, d.internalError(req.Method, err)
	}
	return respBytes, nil
}

func (d *Dispatcher) internalError(method string, err error) error {
	d.logger.Error("failed to dispatch", "method", method, "err", err)
	return internalError
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

func (d *Dispatcher) getNextNonce(address types.Address, number BlockNumber) (uint64, error) {
	if number == PendingBlockNumber {
		res, ok := d.store.GetNonce(address)
		if ok {
			return res, nil
		}
		number = LatestBlockNumber
	}
	header, err := d.getBlockHeaderImpl(number)
	if err != nil {
		return 0, err
	}
	acc, err := d.store.GetAccount(header.StateRoot, address)
	if err != nil {
		return 0, err
	}
	return acc.Nonce, nil
}

func (d *Dispatcher) decodeTxn(arg *txnArgs) (*types.Transaction, error) {
	// set default values
	if arg.From == nil {
		return nil, fmt.Errorf("from is empty")
	}
	if arg.Data != nil && arg.Input != nil {
		return nil, fmt.Errorf("both input and data cannot be set")
	}
	if arg.Nonce == nil {
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
		// use the suggested gas price
		arg.GasPrice = argBytesPtr(d.store.GetAvgGasPrice().Bytes())
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
		// TODO
		arg.Gas = argUintPtr(1000000)
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
