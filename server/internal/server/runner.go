package server

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"

	grpcRuntime "github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pbDaemon "qall-daemon-server/internal/server/protobuf/daemon_runtime_api_v1"
)

func StartServer(ctx context.Context, apiImpl *ApiV1Server, grpcPort string, httpPort string) error {
	lis, err := net.Listen("tcp", ":"+grpcPort)
	if err != nil {
		return fmt.Errorf("failed to listen on gRPC port %s: %w", grpcPort, err)
	}

	grpcServer := grpc.NewServer()
	pbDaemon.RegisterApiServer(grpcServer, apiImpl)

	go func() {
		log.Printf("[gRPC] Server listening on port %s", grpcPort)
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatalf("[gRPC] Fatal server error: %v", err)
		}
	}()

	mux := grpcRuntime.NewServeMux()
	opts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}

	grpcEndpoint := "localhost:" + grpcPort
	if err := pbDaemon.RegisterApiHandlerFromEndpoint(ctx, mux, grpcEndpoint, opts); err != nil {
		return fmt.Errorf("failed to register HTTP Gateway: %w", err)
	}

	log.Printf("[HTTP] REST Gateway listening on port %s", httpPort)
	return http.ListenAndServe(":"+httpPort, mux)
}
