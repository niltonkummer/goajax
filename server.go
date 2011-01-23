package goajax

import (
	"sync"
	"reflect"
	"log"
	"utf8"
	"os"
	"unicode"
	"http"
	"json"
	"strings"
	"strconv"
)

type service struct {
	name   string                 // name of service
	rcvr   reflect.Value          // receiver of methods for the service
	typ    reflect.Type           // type of the receiver
	method map[string]*methodType // registered methods
}


type methodType struct {
	sync.Mutex // protects counters
	method     reflect.Method
	argTypes   []reflect.Type
	returnType reflect.Type
	numCalls   uint
}

type Server struct {
	sync.Mutex // protects the serviceMap
	serviceMap map[string]*service
}


type jsonRequest struct {
	Id      *json.RawMessage  "id"
	Method  string            "method"
	Params  *json.RawMessage  "params"
}

type jsonResponse struct {
	Id      *json.RawMessage  "id"
	Result  interface{}       "result"
	Error   interface{}       "error"
}

func NewServer() *Server {
	s := new(Server)
	s.serviceMap = make(map[string]*service)
	return s
}

// Precompute the reflect type for os.Error.  Can't use os.Error directly
// because Typeof takes an empty interface value.  This is annoying.
var unusedError *os.Error
var typeOfOsError = reflect.Typeof(unusedError).(*reflect.PtrType).Elem()

func (server *Server) register(rcvr interface{}, name string, useName bool) os.Error {
	server.Lock()
	defer server.Unlock()
	
	s := new(service)
	s.typ = reflect.Typeof(rcvr)
	s.rcvr = reflect.NewValue(rcvr)
	sname := reflect.Indirect(s.rcvr).Type().Name()
	if useName {
		sname = name
	}
	if sname == "" {
		log.Exit("rpc: no service name for type", s.typ.String())
	}
	if s.typ.PkgPath() != "" && !isExported(sname) && !useName {
		s := "rpc Register: type " + sname + " is not exported"
		log.Print(s)
		return os.ErrorString(s)
	}
	if _, present := server.serviceMap[sname]; present {
		return os.ErrorString("rpc: service already defined: " + sname)
	}
	s.name = sname
	s.method = make(map[string]*methodType)

	// Install the methods
	MethodLoop: for m := 0; m < s.typ.NumMethod(); m++ {
		method := s.typ.Method(m)
		mtype := method.Type
		mname := method.Name
		if mtype.PkgPath() != "" || !isExported(mname) {
			continue
		}
		
		args := []reflect.Type{}
		
		for i := 1; i < mtype.NumIn(); i++ {
			argType := mtype.In(i)
			if argPointerType, ok := argType.(*reflect.PtrType); ok {
				if argPointerType.Elem().PkgPath() != "" && !isExported(argPointerType.Elem().Name()) {
					log.Println(mname, "argument type not exported:", argPointerType.Elem().Name())
					continue MethodLoop
				}
			}
			args = append(args, argType)
		}
		
		if mtype.NumOut() != 2 {
			log.Println("method", mname, "has wrong number of outs:", mtype.NumOut())
			continue
		}
		
		returnType := mtype.Out(0)
		if returnPointerType, ok := returnType.(*reflect.PtrType); ok {
			if returnPointerType.Elem().PkgPath() != "" && !isExported(returnPointerType.Elem().Name()) {
				log.Println(mname, "return type not exported:", returnPointerType.Elem().Name())
				continue
			}
		}
		
		if errorType := mtype.Out(1); errorType != typeOfOsError {
			log.Println("method", mname, "returns", errorType.String(), "not os.Error")
			continue
		}
		s.method[mname] = &methodType{method: method, argTypes: args, returnType: returnType}
	}

	if len(s.method) == 0 {
		s := "rpc Register: type " + sname + " has no exported methods of suitable type"
		log.Print(s)
		return os.ErrorString(s)
	}
	server.serviceMap[s.name] = s
	return nil
}

func (server *Server) Register(rcvr interface{}) os.Error {
	return server.register(rcvr, "", false)
}

func (server *Server) RegisterName(name string, rcvr interface{}) os.Error {
	return server.register(rcvr, name, true)
}

func _new(t *reflect.PtrType) *reflect.PtrValue {
	v := reflect.MakeZero(t).(*reflect.PtrValue)
	v.PointTo(reflect.MakeZero(t.Elem()))
	return v
}

// Is this an exported - upper case - name?
func isExported(name string) bool {
	rune, _ := utf8.DecodeRuneInString(name)
	return unicode.IsUpper(rune)
}


func (server *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	
	dec := json.NewDecoder(r.Body)
	req := new(jsonRequest)
	err := dec.Decode(req)
	
	if err != nil {
		s := "Invalid JSON-RPC."
		sendError(w, s)
		return
	}
	
	serviceMethod := strings.Split(req.Method, ".", -1)
	server.Lock()
	service, ok := server.serviceMap[serviceMethod[0]]
	server.Unlock()
	
	if !ok {
		s := "Service not found."
		sendError(w, s)
		return	
	}
	
	mtype, ok := service.method[serviceMethod[1]]
	if !ok {
		s := "Method not found."
		sendError(w, s)
		return
	}
	
	args, err := getParams(req, mtype.argTypes)
	
	if err != nil {
		sendError(w, err.String())
		return
	}
		
	args = append([]reflect.Value{service.rcvr}, args...)

	mtype.Lock()
	mtype.numCalls++
	mtype.Unlock()
	function := mtype.method.Func
	
	returnValues := function.Call(args)
	
	// The return value for the method is an os.Error.
	errInter := returnValues[1].Interface()
	errmsg := ""
	if errInter != nil {
		errmsg = errInter.(os.Error).String()
	}
	
	resp := new(jsonResponse)
	
	if errmsg != "" {
		resp.Error = errmsg
	} else {
		resp.Result = returnValues[0].Interface()
	}
	
	resp.Id = req.Id
	
	
	w.SetHeader("Content-Type", "application/json; charset=utf-8")
	enc := json.NewEncoder(w)
	enc.Encode(resp)
}

func sendError(w http.ResponseWriter, s string) {
	w.SetHeader("Content-Type", "application/json; charset=utf-8")
	w.Write([]byte("{\"jsonrpc\": \"2.0\", \"id\":null, \"error\":\"" + s + "\"}"))
}

func getParams(req *jsonRequest, argTypes []reflect.Type) ([]reflect.Value, os.Error) {
	params := make([]*json.RawMessage, 0)
	err := json.Unmarshal(*req.Params, &params)
	
	if err != nil {
		return nil, err
	}
	
	if len(params) != len(argTypes) {
		return nil, os.ErrorString("Incorrect number of parameters.")
	}
	
	args := make([]reflect.Value, 0, len(argTypes))
	
	for i, argType := range argTypes {
		argPointerType, ok := argType.(*reflect.PtrType)
		
		if ok {
				argPointer := reflect.MakeZero(argType).(*reflect.PtrValue)
				argPointer.PointTo(reflect.MakeZero(argPointerType.Elem()))
				err := json.Unmarshal(*params[i], argPointer.Interface())
				if err != nil {
					return nil, os.ErrorString("Type mismatch parameter "+strconv.Itoa(i+1) + ".")
				}
				
				args = append(args, reflect.Value(argPointer))
		} else {
				arg := reflect.MakeZero(argType)
				var v interface{}
				err := json.Unmarshal(*params[i], &v)
				if err != nil {
					return nil, os.ErrorString("Type mismatch parameter "+strconv.Itoa(i+1) + ".")
				}
				value := reflect.NewValue(v)
				if value.Type() == arg.Type() {
					arg.SetValue(value)
				} else if _, ok1 := value.Type().(*reflect.FloatType); ok1 {
					_, ok2 := argType.(*reflect.IntType)
					if ok2 {
						newValue := reflect.NewValue(int(v.(float64)))
						arg.SetValue(newValue)
					} else {
						return nil, os.ErrorString("Type mismatch parameter "+strconv.Itoa(i+1) + ".")
					}
				} else {
					return nil, os.ErrorString("Type mismatch parameter "+strconv.Itoa(i+1) + ".")
				}
				args = append(args, reflect.Value(arg))
		}
	}
	return args, nil
}