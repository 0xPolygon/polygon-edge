package duktape

/*
#cgo !windows CFLAGS: -std=c99 -O3 -Wall -fomit-frame-pointer -fstrict-aliasing -Wno-unused-function
#cgo windows CFLAGS: -O3 -Wall -fomit-frame-pointer -fstrict-aliasing -Wno-unused-function
#cgo linux LDFLAGS: -lm
#cgo freebsd LDFLAGS: -lm
#cgo openbsd LDFLAGS: -lm
#cgo dragonfly LDFLAGS: -lm

#include "duktape.h"
#include <stdbool.h>
#include <stdio.h>

extern duk_size_t goDebugReadFunction(void *uData, char *buffer, duk_size_t length);
extern duk_size_t goDebugWriteFunction(void *uData, char *buffer, duk_size_t length);
extern duk_size_t goDebugPeekFunction(void *uData);
extern void goDebugReadFlushFunction(void *uData);
extern void goDebugWriteFlushFunction(void *uData);
extern duk_idx_t goDebugRequestFunction(duk_context *ctx, void *uData, duk_idx_t nvalues);
extern void goDebugDetachedFunction(duk_context *ctx, void *uData);

static void _duk_debugger_attach(duk_context *ctx, bool peek, bool readFlush, bool writeFlush, bool request, void *uData) {
	duk_debugger_attach(
		ctx,
		goDebugReadFunction,
		((duk_size_t (*)(void*, const char*, duk_size_t)) goDebugWriteFunction),
		peek ? goDebugPeekFunction : NULL,
		readFlush ? goDebugReadFlushFunction : NULL,
		writeFlush ? goDebugWriteFlushFunction : NULL,
		request ? goDebugRequestFunction : NULL,
		goDebugDetachedFunction,
		uData
	);
}
*/
import "C"
import (
	"errors"
	"fmt"
	"reflect"
	"sync"
	"unsafe"
)

type DebugReadFunc = func(uData unsafe.Pointer, buffer []byte) uint
type DebugWriteFunc = func(uData unsafe.Pointer, buffer []byte) uint
type DebugPeekFunc = func(uData unsafe.Pointer) uint
type DebugReadFlushFunc = func(uData unsafe.Pointer)
type DebugWriteFlushFunc = func(uData unsafe.Pointer)
type DebugRequestFunc = func(ctx *Context, uData unsafe.Pointer, nValues int) int
type DebugDetachedFunc = func(ctx *Context, uData unsafe.Pointer)
type DebugNotifyFunc = func(ctx *Context) int

var DukDebuggerMaxAttachments = 64 // arbitrary number of max 64 debugger attachments :)

type attachment struct {
	readFunc       DebugReadFunc
	writeFunc      DebugWriteFunc
	peekFunc       DebugPeekFunc
	readFlushFunc  DebugReadFlushFunc
	writeFlushFunc DebugWriteFlushFunc
	requestFunc    DebugRequestFunc
	detachedFunc   DebugDetachedFunc
	uData          unsafe.Pointer
}

type Debugger struct {
	m           sync.Mutex
	attachments []*attachment
}

var creationMutex = sync.Mutex{}
var debugger *Debugger

// Returns the Duktape debugger instance which can be attached to
// multiple Duktape contexts using the Attach method
func DukDebugger() *Debugger {
	if debugger != nil {
		return debugger
	}
	creationMutex.Lock()
	defer creationMutex.Unlock()
	if debugger != nil {
		return debugger
	}
	debugger = &Debugger{
		m:           sync.Mutex{},
		attachments: make([]*attachment, DukDebuggerMaxAttachments),
	}
	return debugger
}

func (d *Debugger) newAttachment(readFunc DebugReadFunc,
	writeFunc DebugWriteFunc,
	peekFunc DebugPeekFunc,
	readFlushFunc DebugReadFlushFunc,
	writeFlushFunc DebugWriteFlushFunc,
	requestFunc DebugRequestFunc,
	detachedFunc DebugDetachedFunc,
	uData interface{}) (int, error) {

	d.m.Lock()
	defer d.m.Unlock()
	for i := 0; i < len(d.attachments); i++ {
		if d.attachments[i] == nil {
			d.attachments[i] = &attachment{
				readFunc:       readFunc,
				writeFunc:      writeFunc,
				peekFunc:       peekFunc,
				readFlushFunc:  readFlushFunc,
				writeFlushFunc: writeFlushFunc,
				requestFunc:    requestFunc,
				detachedFunc:   detachedFunc,
				uData:          unsafe.Pointer(&uData),
			}
			return i, nil
		}
	}
	return -1, errors.New("no more attachment slots available")
}

func (d *Debugger) removeAttachment(slot int) (*attachment, error) {
	d.m.Lock()
	defer d.m.Unlock()
	if slot < 0 || slot >= len(d.attachments) {
		return nil, errors.New("illegal attachment requested")
	}
	attachment := d.attachments[slot]
	if attachment == nil {
		return nil, errors.New("no attachment registered for requested attachment slot")
	}
	d.attachments[slot] = nil
	return attachment, nil
}

func (d *Debugger) getAttachment(slot int) (*attachment, error) {
	d.m.Lock()
	defer d.m.Unlock()
	if slot < 0 || slot >= len(d.attachments) {
		return nil, errors.New(fmt.Sprintf("illegal attachment requested: %d", slot))
	}
	attachment := d.attachments[slot]
	if attachment == nil {
		return nil, errors.New("no attachment registered for requested attachment slot")
	}
	return attachment, nil
}

// See: http://duktape.org/api.html#duk_debugger_attach
//
// All parameters are optional, except for readFunc, writeFunc.
func (d *Debugger) Attach(ctx *Context,
	readFunc DebugReadFunc,
	writeFunc DebugWriteFunc,
	peekFunc DebugPeekFunc,
	readFlushFunc DebugReadFlushFunc,
	writeFlushFunc DebugWriteFlushFunc,
	requestFunc DebugRequestFunc,
	detachedFunc DebugDetachedFunc,
	uData interface{}) error {

	if readFunc == nil {
		return errors.New("readFunc cannot be nil")
	}
	if writeFunc == nil {
		return errors.New("writeFunc cannot be nil")
	}

	slot, err := d.newAttachment(
		readFunc,
		writeFunc,
		peekFunc,
		readFlushFunc,
		writeFlushFunc,
		requestFunc,
		detachedFunc,
		uData,
	)
	if err != nil {
		return err
	}

	var peek, readFlush, writeFlush, request C.bool = peekFunc != nil,
		readFlushFunc != nil, writeFlushFunc != nil, requestFunc != nil

	dData := slotToPtr(slot)

	C._duk_debugger_attach(
		ctx.duk_context,
		peek,
		readFlush,
		writeFlush,
		request,
		dData,
	)

	return nil
}

// See: http://duktape.org/api.html#duk_debugger_detach
func (d *Debugger) Detach(ctx *Context) {
	C.duk_debugger_detach(ctx.duk_context)
}

// See: http://duktape.org/api.html#duk_debugger_cooperate
func (d *Debugger) Cooperate(ctx *Context) {
	C.duk_debugger_cooperate(ctx.duk_context)
}

// See: http://duktape.org/api.html#duk_debugger_pause
func (d *Debugger) Pause(ctx *Context) {
	C.duk_debugger_pause(ctx.duk_context)
}

// See: http://duktape.org/api.html#duk_debugger_notify
func (d *Debugger) Notify(ctx *Context, notifyFunc DebugNotifyFunc) int {
	nvalues := notifyFunc(ctx)
	return (int)(C.duk_debugger_notify(ctx.duk_context, (C.duk_idx_t)(nvalues)))
}

//export goDebugReadFunction
func goDebugReadFunction(dData unsafe.Pointer, buffer *C.char, length C.duk_size_t) C.duk_size_t {
	a := ptrToAttachment(dData)
	b := ptrToSlice(buffer, length)
	return (C.duk_size_t)(a.readFunc(a.uData, b))
}

//export goDebugWriteFunction
func goDebugWriteFunction(dData unsafe.Pointer, buffer *C.char, length C.duk_size_t) C.duk_size_t {
	a := ptrToAttachment(dData)
	b := ptrToSlice(buffer, length)
	return (C.duk_size_t)(a.writeFunc(a.uData, b))
}

//export goDebugPeekFunction
func goDebugPeekFunction(dData unsafe.Pointer) C.duk_size_t {
	a := ptrToAttachment(dData)
	return (C.duk_size_t)(a.peekFunc(a.uData))
}

//export goDebugReadFlushFunction
func goDebugReadFlushFunction(dData unsafe.Pointer) {
	a := ptrToAttachment(dData)
	a.readFlushFunc(a.uData)
}

//export goDebugWriteFlushFunction
func goDebugWriteFlushFunction(dData unsafe.Pointer) {
	a := ptrToAttachment(dData)
	a.writeFlushFunc(a.uData)
}

//export goDebugRequestFunction
func goDebugRequestFunction(ctx *C.duk_context, dData unsafe.Pointer, nvalues C.duk_idx_t) C.duk_idx_t {
	a := ptrToAttachment(dData)
	d := contextFromPointer(ctx)
	d.transmute(unsafe.Pointer(ctx))
	return (C.duk_idx_t)(a.requestFunc(d, a.uData, int(nvalues)))
}

//export goDebugDetachedFunction
func goDebugDetachedFunction(ctx *C.duk_context, dData unsafe.Pointer) {
	s := ptrToSlot(dData)
	debugger = DukDebugger()
	a, err := debugger.removeAttachment(s)
	if err != nil {
		panic(err)
	}
	defer C.free(dData)
	d := contextFromPointer(ctx)
	d.transmute(unsafe.Pointer(ctx))
	if a.detachedFunc != nil {
		a.detachedFunc(d, a.uData)
	}
}

func ptrToSlice(buffer *C.char, length C.duk_size_t) []byte {
	ptr := uintptr(unsafe.Pointer(buffer))
	l := int(length)
	header := reflect.SliceHeader{Data: ptr, Len: l, Cap: l}
	return *(*[]byte)(unsafe.Pointer(&header))
}

func ptrToSlot(dData unsafe.Pointer) int {
	return int(*(*int32)(dData))
}

func slotToPtr(slot int) unsafe.Pointer {
	s := uint8(slot)
	dData := C.malloc(C.size_t(unsafe.Sizeof(s)))
	*(*C.uint8_t)(dData) = C.uint8_t(s)
	return dData
}

func ptrToAttachment(dData unsafe.Pointer) *attachment {
	slot := ptrToSlot(dData)
	debugger := DukDebugger()
	attachment, err := debugger.getAttachment(slot)
	if err != nil {
		panic(err)
	}
	return attachment
}
