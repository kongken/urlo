package main

import (
	"butterfly.orx.me/core/app"
	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"

	"github.com/kongken/urlo/internal/config"
	"github.com/kongken/urlo/internal/url"
	urlov1 "github.com/kongken/urlo/pkg/proto/urlo/v1"
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
				return nil
			},
		},
	}

	app.New(appConfig).Run()
}
