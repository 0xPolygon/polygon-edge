package sockaddr

import (
	"C"
	"unsafe"

	sockaddrnet "github.com/libp2p/go-sockaddr/net"
)

// AnyToCAny casts a *RawSockaddrAny to a *C.struct_sockaddr_any
func AnyToCAny(a *sockaddrnet.RawSockaddrAny) *C.struct_sockaddr_any {
	return (*C.struct_sockaddr_any)(unsafe.Pointer(a))
}

// CAnyToAny casts a *C.struct_sockaddr_any to a *RawSockaddrAny
func CAnyToAny(a *C.struct_sockaddr_any) *sockaddrnet.RawSockaddrAny {
	return (*sockaddrnet.RawSockaddrAny)(unsafe.Pointer(a))
}
