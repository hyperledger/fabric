// Copyright the Hyperledger Fabric contributors. All rights reserved.
//
// SPDX-License-Identifier: Apache-2.0

// Code generated by protoc-gen-go-grpc. DO NOT EDIT.
// versions:
// - protoc-gen-go-grpc v1.5.1
// - protoc             (unknown)
// source: orderer/clusterserver.proto

package orderer

import (
	context "context"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
)

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
// Requires gRPC-Go v1.64.0 or later.
const _ = grpc.SupportPackageIsVersion9

const (
	ClusterNodeService_Step_FullMethodName = "/orderer.ClusterNodeService/Step"
)

// ClusterNodeServiceClient is the client API for ClusterNodeService service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
//
// Service ClusterNodeService defines communication between cluster members.
type ClusterNodeServiceClient interface {
	// Step passes an implementation-specific message to another cluster member.
	Step(ctx context.Context, opts ...grpc.CallOption) (grpc.BidiStreamingClient[ClusterNodeServiceStepRequest, ClusterNodeServiceStepResponse], error)
}

type clusterNodeServiceClient struct {
	cc grpc.ClientConnInterface
}

func NewClusterNodeServiceClient(cc grpc.ClientConnInterface) ClusterNodeServiceClient {
	return &clusterNodeServiceClient{cc}
}

func (c *clusterNodeServiceClient) Step(ctx context.Context, opts ...grpc.CallOption) (grpc.BidiStreamingClient[ClusterNodeServiceStepRequest, ClusterNodeServiceStepResponse], error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	stream, err := c.cc.NewStream(ctx, &ClusterNodeService_ServiceDesc.Streams[0], ClusterNodeService_Step_FullMethodName, cOpts...)
	if err != nil {
		return nil, err
	}
	x := &grpc.GenericClientStream[ClusterNodeServiceStepRequest, ClusterNodeServiceStepResponse]{ClientStream: stream}
	return x, nil
}

// This type alias is provided for backwards compatibility with existing code that references the prior non-generic stream type by name.
type ClusterNodeService_StepClient = grpc.BidiStreamingClient[ClusterNodeServiceStepRequest, ClusterNodeServiceStepResponse]

// ClusterNodeServiceServer is the server API for ClusterNodeService service.
// All implementations should embed UnimplementedClusterNodeServiceServer
// for forward compatibility.
//
// Service ClusterNodeService defines communication between cluster members.
type ClusterNodeServiceServer interface {
	// Step passes an implementation-specific message to another cluster member.
	Step(grpc.BidiStreamingServer[ClusterNodeServiceStepRequest, ClusterNodeServiceStepResponse]) error
}

// UnimplementedClusterNodeServiceServer should be embedded to have
// forward compatible implementations.
//
// NOTE: this should be embedded by value instead of pointer to avoid a nil
// pointer dereference when methods are called.
type UnimplementedClusterNodeServiceServer struct{}

func (UnimplementedClusterNodeServiceServer) Step(grpc.BidiStreamingServer[ClusterNodeServiceStepRequest, ClusterNodeServiceStepResponse]) error {
	return status.Errorf(codes.Unimplemented, "method Step not implemented")
}
func (UnimplementedClusterNodeServiceServer) testEmbeddedByValue() {}

// UnsafeClusterNodeServiceServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to ClusterNodeServiceServer will
// result in compilation errors.
type UnsafeClusterNodeServiceServer interface {
	mustEmbedUnimplementedClusterNodeServiceServer()
}

func RegisterClusterNodeServiceServer(s grpc.ServiceRegistrar, srv ClusterNodeServiceServer) {
	// If the following call pancis, it indicates UnimplementedClusterNodeServiceServer was
	// embedded by pointer and is nil.  This will cause panics if an
	// unimplemented method is ever invoked, so we test this at initialization
	// time to prevent it from happening at runtime later due to I/O.
	if t, ok := srv.(interface{ testEmbeddedByValue() }); ok {
		t.testEmbeddedByValue()
	}
	s.RegisterService(&ClusterNodeService_ServiceDesc, srv)
}

func _ClusterNodeService_Step_Handler(srv interface{}, stream grpc.ServerStream) error {
	return srv.(ClusterNodeServiceServer).Step(&grpc.GenericServerStream[ClusterNodeServiceStepRequest, ClusterNodeServiceStepResponse]{ServerStream: stream})
}

// This type alias is provided for backwards compatibility with existing code that references the prior non-generic stream type by name.
type ClusterNodeService_StepServer = grpc.BidiStreamingServer[ClusterNodeServiceStepRequest, ClusterNodeServiceStepResponse]

// ClusterNodeService_ServiceDesc is the grpc.ServiceDesc for ClusterNodeService service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var ClusterNodeService_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "orderer.ClusterNodeService",
	HandlerType: (*ClusterNodeServiceServer)(nil),
	Methods:     []grpc.MethodDesc{},
	Streams: []grpc.StreamDesc{
		{
			StreamName:    "Step",
			Handler:       _ClusterNodeService_Step_Handler,
			ServerStreams: true,
			ClientStreams: true,
		},
	},
	Metadata: "orderer/clusterserver.proto",
}
