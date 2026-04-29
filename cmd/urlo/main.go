package main

import (
	"butterfly.orx.me/core/app"
	"github.com/gin-gonic/gin"
)

func main() {
	config := &app.Config{
		Service:   "urlo",
		Namespace: "kongken",
		Router: func(r *gin.Engine) {
			r.GET("/ping", func(c *gin.Context) {
				c.JSON(200, gin.H{"message": "pong"})
			})
		},
	}

	application := app.New(config)
	application.Run()
}
