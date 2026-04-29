package main

import (
	"errors"
	"fmt"
	"log/slog"

	"butterfly.orx.me/core/app"
	butterflys3 "butterfly.orx.me/core/store/s3"
	"github.com/gin-gonic/gin"
	"github.com/kongken/urlo/internal/config"
	"github.com/kongken/urlo/internal/url"
	"github.com/kongken/urlo/internal/url/s3store"
	urlov1 "github.com/kongken/urlo/pkg/proto/urlo/v1"
	"google.golang.org/grpc"
)

func main() {
	svcConfig := &config.ServiceConfig{}
	urlSvc := url.NewService(url.Options{})

	appConfig := &app.Config{
		Service:   "urlo",
		Namespace: "kongken",
		Config:    svcConfig,
		Router: func(r *gin.Engine) {
			r.GET("/ping", func(c *gin.Context) {
				c.JSON(200, gin.H{"message": "pong"})
			})
		},
		GRPCRegister: func(s *grpc.Server) {
			urlov1.RegisterUrlServiceServer(s, urlSvc)
		},
		InitFunc: []func() error{
			func() error {
				urlSvc.SetBaseURL(svcConfig.BaseURL)
				store, err := buildStore(svcConfig.Storage)
				if err != nil {
					return fmt.Errorf("build store: %w", err)
				}
				urlSvc.SetStore(store)
				slog.Info("url service ready", "store", svcConfig.Storage.Driver)
				return nil
			},
		},
	}

	app.New(appConfig).Run()
}

func buildStore(c config.StorageConfig) (url.Store, error) {
	switch c.Driver {
	case "", "memory":
		return url.NewMemoryStore(), nil
	case "s3":
		if c.S3.ConfigName == "" {
			return nil, errors.New("storage.s3.config_name is required")
		}
		client := butterflys3.GetClient(c.S3.ConfigName)
		if client == nil {
			return nil, fmt.Errorf("s3 client %q not found in butterfly store config", c.S3.ConfigName)
		}
		bucket := butterflys3.GetBucket(c.S3.ConfigName)
		if bucket == "" {
			return nil, fmt.Errorf("s3 bucket for %q is empty", c.S3.ConfigName)
		}
		return s3store.New(s3store.Options{
			Client: client,
			Bucket: bucket,
			Prefix: c.S3.Prefix,
		})
	default:
		return nil, fmt.Errorf("unknown storage driver %q", c.Driver)
	}
}
