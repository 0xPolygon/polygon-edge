package jsonrpc

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode"

	"reflect"

	"github.com/umbracle/minimal/minimal"
)

var (
	invalidJSONRequest = &ErrorObject{Code: -32600, Message: "invalid json request"}
	internalError      = &ErrorObject{Code: -32603, Message: "internal error"}
)

func invalidMethod(method string) error {
	return &ErrorObject{Code: -32601, Message: fmt.Sprintf("The method %s does not exist/is not available", method)}
}

func invalidArguments(method string) error {
	return &ErrorObject{Code: -32602, Message: fmt.Sprintf("invalid arguments to %s", method)}
}

type serviceData struct {
	sv      reflect.Value
	funcMap map[string]*funcData
}

type funcData struct {
	inNum int
	reqt  []reflect.Type
	fv    reflect.Value
}

type endpoints struct {
	Eth  *Eth
	Web3 *Web3
	Net  *Net
}

type enabledEndpoints map[string]struct{}

type serverType int

const (
	serverIPC serverType = iota
	serverHTTP
	serverWS
)

// Server is an Ethereum server that handles jsonrpc requests
type Server struct {
	minimal          *minimal.Minimal
	serviceMap       map[string]*serviceData
	endpoints        endpoints
	enabledEndpoints map[serverType]enabledEndpoints
}

func newServer() *Server {
	s := &Server{}

	s.enabledEndpoints[serverIPC] = enabledEndpoints{}
	s.enabledEndpoints[serverHTTP] = enabledEndpoints{}
	s.enabledEndpoints[serverWS] = enabledEndpoints{}

	return s
}

func (s *Server) registerEndpoints() {
	s.endpoints.Eth = &Eth{s}
	s.endpoints.Net = &Net{s}
	s.endpoints.Web3 = &Web3{s}

	s.registerService("eth", s.endpoints.Eth)
	s.registerService("net", s.endpoints.Net)
	s.registerService("web3", s.endpoints.Web3)
}

func (s *Server) handle(reqBody []byte) ([]byte, error) {
	var req Request
	if err := json.Unmarshal(reqBody, &req); err != nil {
		return nil, invalidJSONRequest
	}
	var params []interface{}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return nil, invalidJSONRequest
	}

	callName := strings.SplitN(req.Method, "_", 2)
	if len(callName) != 2 {
		return nil, invalidMethod(req.Method)
	}

	serviceName, funcName := callName[0], callName[1]
	service, ok := s.serviceMap[serviceName]
	if !ok {
		return nil, invalidMethod(req.Method)
	}
	fd, ok := service.funcMap[funcName]
	if !ok {
		return nil, invalidMethod(req.Method)
	}

	if len(params) != fd.inNum-1 {
		return nil, invalidArguments(req.Method)
	}

	inArgs := make([]reflect.Value, fd.inNum)
	inArgs[0] = service.sv

	for i := 0; i < fd.inNum-1; i++ {
		elem := reflect.ValueOf(params[i])
		if elem.Type() != fd.reqt[i+1] {
			return nil, invalidArguments(req.Method)
		}
		inArgs[i+1] = elem
	}

	output := fd.fv.Call(inArgs)
	err := getError(output[1])
	if err != nil {
		return nil, internalError
	}

	var data []byte
	res := output[0].Interface()
	if res != nil {
		data, err = json.Marshal(res)
		if err != nil {
			return nil, internalError
		}
	}

	resp := Response{
		Result: data,
	}
	respBytes, err := json.Marshal(resp)
	if err != nil {
		return nil, internalError
	}
	return respBytes, nil
}

func (s *Server) registerService(serviceName string, service interface{}) {
	if serviceName == "" {
		panic(fmt.Sprintf("jsonrpc: serviceName cannot be empty"))
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
		funcMap[name] = fd
	}

	s.serviceMap[serviceName] = &serviceData{
		sv:      reflect.ValueOf(service),
		funcMap: funcMap,
	}
}

func removePtr(t reflect.Type) reflect.Type {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t
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

func (s *Server) funcExample(b string) (interface{}, error) {
	return nil, nil
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
