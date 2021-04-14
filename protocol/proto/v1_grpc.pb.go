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

// V1Client is the client API for V1 service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type V1Client interface {
	GetCurrent(ctx context.Context, in *empty.Empty, opts ...grpc.CallOption) (*V1Status, error)
	GetObjectsByHash(ctx context.Context, in *HashRequest, opts ...grpc.CallOption) (*Response, error)
	GetHeaders(ctx context.Context, in *GetHeadersRequest, opts ...grpc.CallOption) (*Response, error)
	Notify(ctx context.Context, in *NotifyReq, opts ...grpc.CallOption) (*empty.Empty, error)
	Watch(ctx context.Context, in *empty.Empty, opts ...grpc.CallOption) (V1_WatchClient, error)
}

type v1Client struct {
	cc grpc.ClientConnInterface
}

func NewV1Client(cc grpc.ClientConnInterface) V1Client {
	return &v1Client{cc}
}

func (c *v1Client) GetCurrent(ctx context.Context, in *empty.Empty, opts ...grpc.CallOption) (*V1Status, error) {
	out := new(V1Status)
	err := c.cc.Invoke(ctx, "/v1.V1/GetCurrent", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *v1Client) GetObjectsByHash(ctx context.Context, in *HashRequest, opts ...grpc.CallOption) (*Response, error) {
	out := new(Response)
	err := c.cc.Invoke(ctx, "/v1.V1/GetObjectsByHash", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *v1Client) GetHeaders(ctx context.Context, in *GetHeadersRequest, opts ...grpc.CallOption) (*Response, error) {
	out := new(Response)
	err := c.cc.Invoke(ctx, "/v1.V1/GetHeaders", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *v1Client) Notify(ctx context.Context, in *NotifyReq, opts ...grpc.CallOption) (*empty.Empty, error) {
	out := new(empty.Empty)
	err := c.cc.Invoke(ctx, "/v1.V1/Notify", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *v1Client) Watch(ctx context.Context, in *empty.Empty, opts ...grpc.CallOption) (V1_WatchClient, error) {
	stream, err := c.cc.NewStream(ctx, &V1_ServiceDesc.Streams[0], "/v1.V1/Watch", opts...)
	if err != nil {
		return nil, err
	}
	x := &v1WatchClient{stream}
	if err := x.ClientStream.SendMsg(in); err != nil {
		return nil, err
	}
	if err := x.ClientStream.CloseSend(); err != nil {
		return nil, err
	}
	return x, nil
}

type V1_WatchClient interface {
	Recv() (*V1Status, error)
	grpc.ClientStream
}

type v1WatchClient struct {
	grpc.ClientStream
}

func (x *v1WatchClient) Recv() (*V1Status, error) {
	m := new(V1Status)
	if err := x.ClientStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

// V1Server is the server API for V1 service.
// All implementations must embed UnimplementedV1Server
// for forward compatibility
type V1Server interface {
	GetCurrent(context.Context, *empty.Empty) (*V1Status, error)
	GetObjectsByHash(context.Context, *HashRequest) (*Response, error)
	GetHeaders(context.Context, *GetHeadersRequest) (*Response, error)
	Notify(context.Context, *NotifyReq) (*empty.Empty, error)
	Watch(*empty.Empty, V1_WatchServer) error
	mustEmbedUnimplementedV1Server()
}

// UnimplementedV1Server must be embedded to have forward compatible implementations.
type UnimplementedV1Server struct {
}

func (UnimplementedV1Server) GetCurrent(context.Context, *empty.Empty) (*V1Status, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetCurrent not implemented")
}
func (UnimplementedV1Server) GetObjectsByHash(context.Context, *HashRequest) (*Response, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetObjectsByHash not implemented")
}
func (UnimplementedV1Server) GetHeaders(context.Context, *GetHeadersRequest) (*Response, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetHeaders not implemented")
}
func (UnimplementedV1Server) Notify(context.Context, *NotifyReq) (*empty.Empty, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Notify not implemented")
}
func (UnimplementedV1Server) Watch(*empty.Empty, V1_WatchServer) error {
	return status.Errorf(codes.Unimplemented, "method Watch not implemented")
}
func (UnimplementedV1Server) mustEmbedUnimplementedV1Server() {}

// UnsafeV1Server may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to V1Server will
// result in compilation errors.
type UnsafeV1Server interface {
	mustEmbedUnimplementedV1Server()
}

func RegisterV1Server(s grpc.ServiceRegistrar, srv V1Server) {
	s.RegisterService(&V1_ServiceDesc, srv)
}

func _V1_GetCurrent_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(empty.Empty)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(V1Server).GetCurrent(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/v1.V1/GetCurrent",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(V1Server).GetCurrent(ctx, req.(*empty.Empty))
	}
	return interceptor(ctx, in, info, handler)
}

func _V1_GetObjectsByHash_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(HashRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(V1Server).GetObjectsByHash(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/v1.V1/GetObjectsByHash",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(V1Server).GetObjectsByHash(ctx, req.(*HashRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _V1_GetHeaders_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetHeadersRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(V1Server).GetHeaders(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/v1.V1/GetHeaders",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(V1Server).GetHeaders(ctx, req.(*GetHeadersRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _V1_Notify_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(NotifyReq)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(V1Server).Notify(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/v1.V1/Notify",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(V1Server).Notify(ctx, req.(*NotifyReq))
	}
	return interceptor(ctx, in, info, handler)
}

func _V1_Watch_Handler(srv interface{}, stream grpc.ServerStream) error {
	m := new(empty.Empty)
	if err := stream.RecvMsg(m); err != nil {
		return err
	}
	return srv.(V1Server).Watch(m, &v1WatchServer{stream})
}

type V1_WatchServer interface {
	Send(*V1Status) error
	grpc.ServerStream
}

type v1WatchServer struct {
	grpc.ServerStream
}

func (x *v1WatchServer) Send(m *V1Status) error {
	return x.ServerStream.SendMsg(m)
}

// V1_ServiceDesc is the grpc.ServiceDesc for V1 service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var V1_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "v1.V1",
	HandlerType: (*V1Server)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "GetCurrent",
			Handler:    _V1_GetCurrent_Handler,
		},
		{
			MethodName: "GetObjectsByHash",
			Handler:    _V1_GetObjectsByHash_Handler,
		},
		{
			MethodName: "GetHeaders",
			Handler:    _V1_GetHeaders_Handler,
		},
		{
			MethodName: "Notify",
			Handler:    _V1_Notify_Handler,
		},
	},
	Streams: []grpc.StreamDesc{
		{
			StreamName:    "Watch",
			Handler:       _V1_Watch_Handler,
			ServerStreams: true,
		},
	},
	Metadata: "protocol/proto/v1.proto",
}
