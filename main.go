package main

import (
	"csye7255-project-one/config"
	"csye7255-project-one/middleware"
	"csye7255-project-one/routes"
	"fmt"
	"log"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatalf("Error loading .env file")
	}

	// Initialize Redis
	config.SetupRedis()

	// Initialize Google JWT public certificates for token validation
	config.InitGoogleCerts()

	// Initialize the Gin router
	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()
	r.SetTrustedProxies([]string{})

	// Apply AuthMiddleware to secure all routes
	r.Use(middleware.AuthMiddleware())

	// Set up routes
	routes.SetupRoutes(r)

	// Start the server
	if err := r.Run(fmt.Sprintf(":%s", os.Getenv("PORT"))); err != nil {
		fmt.Printf("Failed to start server: %v\n", err)
	}
}
