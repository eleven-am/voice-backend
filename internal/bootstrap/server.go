package bootstrap

import (
	"context"
	"errors"
	"net/http"

	"github.com/eleven-am/voice-backend/internal/gateway"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"go.uber.org/fx"
)

func corsConfig(origins []string) middleware.CORSConfig {
	cfg := middleware.CORSConfig{
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
		ExposeHeaders: []string{
			"X-Session-Id",
		},
		MaxAge: 86400,
	}

	if len(origins) > 0 {
		cfg.AllowOrigins = origins
		cfg.AllowCredentials = true
	} else {
		cfg.AllowOrigins = []string{"*"}
		cfg.AllowCredentials = false
	}

	return cfg
}

func NewEchoServer(cfg *Config) *echo.Echo {
	e := echo.New()
	e.HideBanner = true
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(middleware.CORSWithConfig(corsConfig(cfg.CORSOrigins)))
	return e
}

func StartServer(lc fx.Lifecycle, e *echo.Echo, cfg *Config) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			go func() {
				if err := e.Start(cfg.ServerAddr); err != nil && !errors.Is(err, http.ErrServerClosed) {
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
		gateway.Module,
		HandlersModule,
		VoiceModule,
		HealthModule,
	).Run()
}
