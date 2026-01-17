package bootstrap

import (
	"context"
	"net/http"

	"github.com/eleven-am/voice-backend/internal/gateway"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"go.uber.org/fx"
)

var defaultCORSConfig = middleware.CORSConfig{
	AllowOrigins: []string{"*"},
	AllowMethods: []string{
		http.MethodGet,
		http.MethodHead,
		http.MethodPut,
		http.MethodPatch,
		http.MethodPost,
		http.MethodDelete,
		http.MethodOptions,
	},
	AllowHeaders: []string{
		"Accept",
		"Authorization",
		"Content-Type",
		"X-Requested-With",
		"X-CSRF-Token",
	},
	AllowCredentials: true,
	MaxAge:           86400,
}

func NewEchoServer() *echo.Echo {
	e := echo.New()
	e.HideBanner = true
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(middleware.CORSWithConfig(defaultCORSConfig))
	return e
}

func StartServer(lc fx.Lifecycle, e *echo.Echo, cfg *Config) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			go func() {
				if err := e.Start(cfg.ServerAddr); err != nil && err != http.ErrServerClosed {
					e.Logger.Fatal(err)
				}
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			return e.Shutdown(ctx)
		},
	})
}

var ServerModule = fx.Options(
	fx.Provide(NewEchoServer),
	fx.Invoke(StartServer),
)

func Run() {
	fx.New(
		fx.Provide(LoadConfig),
		InfrastructureModule,
		StoresModule,
		ServerModule,
		GRPCModule,
		gateway.Module,
		HandlersModule,
	).Run()
}
