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

// IbftOperatorClient is the client API for IbftOperator service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type IbftOperatorClient interface {
	GetSnapshot(ctx context.Context, in *SnapshotReq, opts ...grpc.CallOption) (*Snapshot, error)
	Propose(ctx context.Context, in *Candidate, opts ...grpc.CallOption) (*empty.Empty, error)
	Candidates(ctx context.Context, in *empty.Empty, opts ...grpc.CallOption) (*CandidatesResp, error)
	Status(ctx context.Context, in *empty.Empty, opts ...grpc.CallOption) (*IbftStatusResp, error)
}

type ibftOperatorClient struct {
	cc grpc.ClientConnInterface
}

func NewIbftOperatorClient(cc grpc.ClientConnInterface) IbftOperatorClient {
	return &ibftOperatorClient{cc}
}

func (c *ibftOperatorClient) GetSnapshot(ctx context.Context, in *SnapshotReq, opts ...grpc.CallOption) (*Snapshot, error) {
	out := new(Snapshot)
	err := c.cc.Invoke(ctx, "/v1.IbftOperator/GetSnapshot", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *ibftOperatorClient) Propose(ctx context.Context, in *Candidate, opts ...grpc.CallOption) (*empty.Empty, error) {
	out := new(empty.Empty)
	err := c.cc.Invoke(ctx, "/v1.IbftOperator/Propose", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *ibftOperatorClient) Candidates(ctx context.Context, in *empty.Empty, opts ...grpc.CallOption) (*CandidatesResp, error) {
	out := new(CandidatesResp)
	err := c.cc.Invoke(ctx, "/v1.IbftOperator/Candidates", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *ibftOperatorClient) Status(ctx context.Context, in *empty.Empty, opts ...grpc.CallOption) (*IbftStatusResp, error) {
	out := new(IbftStatusResp)
	err := c.cc.Invoke(ctx, "/v1.IbftOperator/Status", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// IbftOperatorServer is the server API for IbftOperator service.
// All implementations must embed UnimplementedIbftOperatorServer
// for forward compatibility
type IbftOperatorServer interface {
	GetSnapshot(context.Context, *SnapshotReq) (*Snapshot, error)
	Propose(context.Context, *Candidate) (*empty.Empty, error)
	Candidates(context.Context, *empty.Empty) (*CandidatesResp, error)
	Status(context.Context, *empty.Empty) (*IbftStatusResp, error)
	mustEmbedUnimplementedIbftOperatorServer()
}

// UnimplementedIbftOperatorServer must be embedded to have forward compatible implementations.
type UnimplementedIbftOperatorServer struct {
}

func (UnimplementedIbftOperatorServer) GetSnapshot(context.Context, *SnapshotReq) (*Snapshot, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetSnapshot not implemented")
}
func (UnimplementedIbftOperatorServer) Propose(context.Context, *Candidate) (*empty.Empty, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Propose not implemented")
}
func (UnimplementedIbftOperatorServer) Candidates(context.Context, *empty.Empty) (*CandidatesResp, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Candidates not implemented")
}
func (UnimplementedIbftOperatorServer) Status(context.Context, *empty.Empty) (*IbftStatusResp, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Status not implemented")
}
func (UnimplementedIbftOperatorServer) mustEmbedUnimplementedIbftOperatorServer() {}

// UnsafeIbftOperatorServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to IbftOperatorServer will
// result in compilation errors.
type UnsafeIbftOperatorServer interface {
	mustEmbedUnimplementedIbftOperatorServer()
}

func RegisterIbftOperatorServer(s grpc.ServiceRegistrar, srv IbftOperatorServer) {
	s.RegisterService(&IbftOperator_ServiceDesc, srv)
}

func _IbftOperator_GetSnapshot_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(SnapshotReq)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(IbftOperatorServer).GetSnapshot(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/v1.IbftOperator/GetSnapshot",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(IbftOperatorServer).GetSnapshot(ctx, req.(*SnapshotReq))
	}
	return interceptor(ctx, in, info, handler)
}

func _IbftOperator_Propose_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(Candidate)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(IbftOperatorServer).Propose(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/v1.IbftOperator/Propose",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(IbftOperatorServer).Propose(ctx, req.(*Candidate))
	}
	return interceptor(ctx, in, info, handler)
}

func _IbftOperator_Candidates_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(empty.Empty)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(IbftOperatorServer).Candidates(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/v1.IbftOperator/Candidates",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(IbftOperatorServer).Candidates(ctx, req.(*empty.Empty))
	}
	return interceptor(ctx, in, info, handler)
}

func _IbftOperator_Status_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(empty.Empty)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(IbftOperatorServer).Status(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/v1.IbftOperator/Status",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(IbftOperatorServer).Status(ctx, req.(*empty.Empty))
	}
	return interceptor(ctx, in, info, handler)
}

// IbftOperator_ServiceDesc is the grpc.ServiceDesc for IbftOperator service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var IbftOperator_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "v1.IbftOperator",
	HandlerType: (*IbftOperatorServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "GetSnapshot",
			Handler:    _IbftOperator_GetSnapshot_Handler,
		},
		{
			MethodName: "Propose",
			Handler:    _IbftOperator_Propose_Handler,
		},
		{
			MethodName: "Candidates",
			Handler:    _IbftOperator_Candidates_Handler,
		},
		{
			MethodName: "Status",
			Handler:    _IbftOperator_Status_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "consensus/ibft2/proto/operator.proto",
}
