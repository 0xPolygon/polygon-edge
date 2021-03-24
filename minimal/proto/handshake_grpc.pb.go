// Code generated by protoc-gen-go-grpc. DO NOT EDIT.

package proto

import (
	context "context"
	empty "github.com/golang/protobuf/ptypes/empty"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
)

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
// Requires gRPC-Go v1.32.0 or later.
const _ = grpc.SupportPackageIsVersion7

// HandshakeClient is the client API for Handshake service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type HandshakeClient interface {
	Hello(ctx context.Context, in *HelloReq, opts ...grpc.CallOption) (*empty.Empty, error)
}

type handshakeClient struct {
	cc grpc.ClientConnInterface
}

func NewHandshakeClient(cc grpc.ClientConnInterface) HandshakeClient {
	return &handshakeClient{cc}
}

func (c *handshakeClient) Hello(ctx context.Context, in *HelloReq, opts ...grpc.CallOption) (*empty.Empty, error) {
	out := new(empty.Empty)
	err := c.cc.Invoke(ctx, "/v1.Handshake/Hello", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// HandshakeServer is the server API for Handshake service.
// All implementations must embed UnimplementedHandshakeServer
// for forward compatibility
type HandshakeServer interface {
	Hello(context.Context, *HelloReq) (*empty.Empty, error)
	mustEmbedUnimplementedHandshakeServer()
}

// UnimplementedHandshakeServer must be embedded to have forward compatible implementations.
type UnimplementedHandshakeServer struct {
}

func (UnimplementedHandshakeServer) Hello(context.Context, *HelloReq) (*empty.Empty, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Hello not implemented")
}
func (UnimplementedHandshakeServer) mustEmbedUnimplementedHandshakeServer() {}

// UnsafeHandshakeServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to HandshakeServer will
// result in compilation errors.
type UnsafeHandshakeServer interface {
	mustEmbedUnimplementedHandshakeServer()
}

func RegisterHandshakeServer(s grpc.ServiceRegistrar, srv HandshakeServer) {
	s.RegisterService(&Handshake_ServiceDesc, srv)
}

func _Handshake_Hello_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(HelloReq)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(HandshakeServer).Hello(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/v1.Handshake/Hello",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(HandshakeServer).Hello(ctx, req.(*HelloReq))
	}
	return interceptor(ctx, in, info, handler)
}

// Handshake_ServiceDesc is the grpc.ServiceDesc for Handshake service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var Handshake_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "v1.Handshake",
	HandlerType: (*HandshakeServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "Hello",
			Handler:    _Handshake_Hello_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "minimal/proto/handshake.proto",
}
