//go:build wasip1

package main

import (
	"encoding/json"
	"errors"
	goruntime "runtime"
	"unsafe"
)

//go:wasmimport dian115 host_call
func hostCall(ptr, length uint32) uint32

//go:wasmimport dian115 host_read
func hostRead(ptr, capacity uint32) uint32

type wasmHost struct{}

func (wasmHost) Call(method string, params, target any) error {
	request, err := json.Marshal(map[string]any{"method": method, "params": params})
	if err != nil {
		return err
	}
	size := hostCall(uint32(uintptr(unsafe.Pointer(&request[0]))), uint32(len(request)))
	goruntime.KeepAlive(request)
	if size == 0 || size > 16<<20 {
		return errors.New("invalid host response size")
	}
	response := make([]byte, size)
	if hostRead(uint32(uintptr(unsafe.Pointer(&response[0]))), size) != size {
		return errors.New("host response read failed")
	}
	var envelope struct {
		Result json.RawMessage `json:"result"`
		Error  string          `json:"error"`
	}
	if err = json.Unmarshal(response, &envelope); err != nil {
		return err
	}
	if envelope.Error != "" {
		return errors.New("宿主调用被拒绝或无法完成，请查看宿主审计记录")
	}
	if target == nil {
		return nil
	}
	return json.Unmarshal(envelope.Result, target)
}

var inputBuffer, outputBuffer []byte
var guest = &Runtime{host: wasmHost{}}

//go:wasmexport dian115_alloc
func allocate(size uint32) uint32 {
	if size == 0 || size > 16<<20 {
		panic("invalid input size")
	}
	inputBuffer = make([]byte, size)
	return uint32(uintptr(unsafe.Pointer(&inputBuffer[0])))
}

//go:wasmexport dian115_handle
func handle(ptr, length uint32) uint64 {
	if length == 0 || length > 16<<20 {
		panic("invalid invocation size")
	}
	var request RPC
	var result any
	var err error
	raw := unsafe.Slice((*byte)(unsafe.Pointer(uintptr(ptr))), int(length))
	if json.Unmarshal(raw, &request) != nil {
		err = errors.New("invalid invocation")
	} else {
		result, err = guest.Handle(request)
	}
	response := map[string]any{"result": result}
	if err != nil {
		response = map[string]any{"error": map[string]any{"code": -32602, "message": err.Error()}}
	}
	outputBuffer, _ = json.Marshal(response)
	return uint64(uintptr(unsafe.Pointer(&outputBuffer[0])))<<32 | uint64(len(outputBuffer))
}
func main() {}
