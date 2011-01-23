package goajax

import (
	"testing"
	"os"
	"http"
	"strings"
	"bufio"
	"bytes"
	"io"
	"json"
	"strconv"
)

type TestService int

func (s *TestService) Add(a, b float64) (float64, os.Error) {
	return a + b, nil
}

func (s *TestService) Repeat(obj *A) (string, os.Error) {
	out := ""
	for i := 0; i < obj.Y; i++ {
		out += obj.X
	}
	return out, nil
}

func (s *TestService) ObjAdd(obj1, obj2 *A) (*A, os.Error) {
	out := new(A)
	out.X = obj1.X + obj2.X
	out.Y = obj1.Y + obj2.Y
	
	return out, nil
}

func (s *TestService) Unrepeat(in string) (*A, os.Error) {
	runes := []int(in)
	j := -1
	
	for i := 1; i < int(len(runes)/2); i++ {
		if string(runes[0:i]) == string(runes[i:i*2]) {
			j = i
			break
		}
	}
	out := new(A)
	if j > 0 {
		out.X = string(runes[0:j])
		out.Y = int(len(runes) / j)
	} else {
		out.X = in
		out.Y = 1
	}
	
	return out, nil
}

func TestRegistering(t *testing.T) {
	s := NewServer()
	s.Register(new(TestService))
}

func TestRegisteringWithName(t *testing.T) {
	s := NewServer()
	s.RegisterName("service", new(TestService))
}

type test struct {
	req       string
	resp  interface{}
	error interface{}
}

var tests = []test {
	test{req: `{"jsonrpc": "2.0", "method":"TestService.Add","params":[40, 2], "id":0}`, resp: 42.00, error: nil},
	test{req: `{"jsonrpc": "2.0", "method":"TestService.NonExistent","params":[40, 2], "id":0}`, resp: nil, error: "Method not found."},
	test{req: `{"jsonrpc": "2.0", "method":"OtherService.Add","params":[40, 2], "id":0}`, resp: nil, error: "Service not found."},
	test{req: `{"jsonrpc": "2.0", "method":"TestService.Add","params":[1, 2.23], "id":0}`, resp: 3.23, error: nil},
	test{req: `{"jsonrpc": "2.0", "method":"TestService.Add","params":[40, 2], "id":0`, resp: nil, error: "Invalid JSON-RPC."},
	test{req: `{"jsonrpc": "2.0", "method":"TestService.Repeat","params":[{"x": "str", "y": 3}], "id":0}`, resp: "strstrstr", error: nil},
	test{req: `{"jsonrpc": "2.0", "method":"TestService.Repeat","params":["str"], "id":0}`, resp: nil, error: "Type mismatch parameter 1."},
	test{req: `{"jsonrpc": "2.0", "method":"TestService.Unrepeat","params":["strstrstr"], "id":0}`, resp: map[string]interface{}{"x":"str", "y":3}, error: nil},
	test{req: `{"jsonrpc": "2.0", "method":"TestService.ObjAdd","params":[{"x": "my", "y": 4}, {"x": "str", "y": 3}], "id":0}`, resp: map[string]interface{}{"x":"mystr", "y":7}, error: nil},
	test{req: `{"jsonrpc": "2.0", "method":"TestService.ObjAdd","params":[{"x": "my", "y": 4}], "id":0}`, resp: nil, error: "Incorrect number of parameters."},
	test{req: `{"jsonrpc": "2.0", "method":"TestService.ObjAdd","params":[], "id":0}`, resp: nil, error: "Incorrect number of parameters."},
}

type A struct {
	X   string   "x"
	Y   int      "y"
}

func TestCall(t *testing.T) {
	s := NewServer()
	s.Register(new(TestService))
	
	for i, test := range tests {
		str := "POST /json HTTP/1.1\nContent-Length: " + strconv.Itoa(len(test.req)) + "\n\n"+test.req
		r := bufio.NewReader(strings.NewReader(str))
	
		req, _ := http.ReadRequest(r)
		b := bytes.NewBuffer([]byte{})
		w := &TestResponseWriter{buffer: b}
		s.ServeHTTP(w, req)
		resp := new(jsonResponse)
		json.Unmarshal(b.Bytes(), resp)
		
		if test.error != nil {
			if resp.Error == nil {
				t.Error("Test", i, "Error not present")
				t.Fail()
				return
			} else {
				if test.error.(string) != resp.Error.(string) {
					t.Error("Test", i, resp.Error.(string))
					t.Fail()
					return
				}
			}
		} else {
			if resp.Error != nil {
				t.Error("Test", i, resp.Error.(string))
				t.Fail()
				return
			}
		}
		if test.resp == nil && resp.Result == nil {
			continue
		}
		
		switch test.resp.(type) {
			case float64:
				if fValue, ok := resp.Result.(float64); !ok || fValue != test.resp.(float64) {
					t.Error("Test", i, "Did not match float")
					t.Fail()
					return
				}
			case int:
				if iValue, ok := resp.Result.(int); !ok || iValue != test.resp.(int) {
					t.Error("Test", i, "Did not match int")
					t.Fail()
					return
				}
			case bool:
				if bValue, ok := resp.Result.(bool); !ok || bValue != test.resp.(bool) {
					t.Error("Test", i, "Did not match bool")
					t.Fail()
					return
				}
			case string:
				if sValue, ok := resp.Result.(string); !ok || sValue != test.resp.(string) {
					t.Error("Test", i, "Did not match string")
					t.Fail()
					return
				}
			case map[string]interface{}:
				mapValue, ok := resp.Result.(map[string]interface{})
				if !ok {
					t.Error("Test", i, "Result was not a map[string]interface{}")
					t.Fail()
					return
				}
				mapResult :=  test.resp.(map[string]interface{})
				if mapValue["x"].(string) != mapResult["x"].(string) || int(mapValue["y"].(float64)) != mapResult["y"].(int) {
					t.Error("Test", i, "Did not match object")
					t.Fail()
					return
				} 
			default:
				t.Error("Test", i, "Unknown result")
				t.Fail()
				return
		}
	}
}

type TestResponseWriter struct {
	buffer *bytes.Buffer
}

func (t *TestResponseWriter) RemoteAddr() string {
	return "127.0.0.1"
}
func (t *TestResponseWriter) UsingTLS() bool {
	return false
}
func (t *TestResponseWriter) SetHeader(k, v string) {

}
func (t *TestResponseWriter) Write(p []byte) (int, os.Error) {
	return t.buffer.Write(p)
}
func (t *TestResponseWriter) WriteHeader(i int) {

}
func (t *TestResponseWriter) Flush() {

}
func (t *TestResponseWriter) Hijack() (io.ReadWriteCloser, *bufio.ReadWriter, os.Error) {
	return nil, nil, nil
}