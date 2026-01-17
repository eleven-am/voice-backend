package bootstrap

import (
	"context"
	"log/slog"
	"net"

	"github.com/eleven-am/voice-backend/internal/gateway"
	pb "github.com/eleven-am/voice-backend/internal/gateway/proto"
	"go.uber.org/fx"
	"google.golang.org/grpc"
)

func NewGRPCServer() *grpc.Server {
	return grpc.NewServer()
}

func RegisterGatewayService(server *grpc.Server, gatewayServer *gateway.GRPCServer) {
	pb.RegisterVoiceGatewayServer(server, gatewayServer)
}

func StartGRPCServer(lc fx.Lifecycle, server *grpc.Server, cfg *Config, logger *slog.Logger) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			lis, err := net.Listen("tcp", cfg.GRPCAddr)
			if err != nil {
				return err
			}
			go func() {
				logger.Info("gRPC server starting", "addr", cfg.GRPCAddr)
				if err := server.Serve(lis); err != nil {
					logger.Error("gRPC server error", "error", err)
				}
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			server.GracefulStop()
			return nil
		},
	})
}

var GRPCModule = fx.Options(
	fx.Provide(NewGRPCServer),
	fx.Invoke(RegisterGatewayService),
	fx.Invoke(StartGRPCServer),
)
