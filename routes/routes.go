package routes

import (
	"csye7255-project-one/controllers"

	"github.com/gin-gonic/gin"
)

func SetupRoutes(router *gin.Engine) {
	v1 := router.Group("/v1")
	{
		plans := v1.Group("/plans")
		{
			plans.POST("/", controllers.CreateRecord)
			plans.GET("/:id", controllers.GetRecord)
			plans.DELETE("/:id", controllers.DeleteRecord)
			plans.PATCH("/:id", controllers.PatchRecord)
			plans.PUT("/:id", controllers.PutRecord)
		}
	}
}
