package main // or package server

import (
	pb "HLL-BTP/server"
	"HLL-BTP/types/hll"
	"context"
	"log"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// server struct implements the gRPC HllServiceServer interface
type server struct {
	pb.UnimplementedHllServiceServer                // Must embed this
	hllInstance                      *hll.Hllpp_set // Holds the single, thread-safe HLL++ instance
}

// NewServer creates a new gRPC server
func NewServer() *server {
	return &server{
		// Initialize HLL++ in concurrent mode
		hllInstance: hll.GetHLLPP(true),
	}
}

// Insert is the RPC handler for inserting an IP
func (s *server) Insert(ctx context.Context, req *pb.InsertRequest) (*pb.InsertResponse, error) {
	if req.Ip == "" {
		return nil, status.Error(codes.InvalidArgument, "IP cannot be empty")
	}

	if err := s.hllInstance.Insert(req.Ip); err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to insert: %v", err)
	}

	return &pb.InsertResponse{}, nil
}

// GetEstimate is the RPC handler for getting the count
func (s *server) GetEstimate(ctx context.Context, req *pb.GetEstimateRequest) (*pb.GetEstimateResponse, error) {
	estimate := s.hllInstance.GetElements()
	return &pb.GetEstimateResponse{Estimate: estimate}, nil
}

// Reset is the RPC handler for resetting the sketch
func (s *server) Reset(ctx context.Context, req *pb.ResetRequest) (*pb.ResetResponse, error) {
	s.hllInstance.Reset()
	return &pb.ResetResponse{}, nil
}

// GetSketch is the RPC handler for serializing and sending the sketch
func (s *server) GetSketch(ctx context.Context, req *pb.GetSketchRequest) (*pb.Sketch, error) {
	sketchMsg, err := s.hllInstance.ExportSketch()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to export sketch: %v", err)
	}
	return sketchMsg, nil
}

// MergeSketch is the RPC handler for receiving and merging another sketch
func (s *server) MergeSketch(ctx context.Context, req *pb.MergeRequest) (*pb.MergeResponse, error) {
	tempHLL, err := hll.NewHllppSetFromSketch(req.Sketch)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "Failed to decode sketch: %v", err)
	}

	if err := s.hllInstance.MergeSets(tempHLL); err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to merge sketch: %v", err)
	}

	return &pb.MergeResponse{}, nil
}

// Health is the RPC handler for health checks
func (s *server) Health(ctx context.Context, req *pb.HealthRequest) (*pb.HealthResponse, error) {
	// Simple health check: always returns SERVING
	return &pb.HealthResponse{Status: pb.HealthResponse_SERVING}, nil
}

func main() {
	lis, err := net.Listen("tcp", ":8080") // Listen on port 8080
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	grpcServer := grpc.NewServer()
	pb.RegisterHllServiceServer(grpcServer, NewServer())

	log.Println("gRPC server listening on :8080")
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}
