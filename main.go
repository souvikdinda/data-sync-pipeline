package main

import (
	"csye7255-project-one/config"
	"csye7255-project-one/middleware"
	"csye7255-project-one/routes"
	"csye7255-project-one/services"
	"fmt"
	"log"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

var (
	indexName = "plans"
	queueName = "plan_requests"
)

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatalf("Error loading .env file")
	}

	// Initialize Redis
	config.SetupRedis()

	// Initialize RabbitMQ
	config.SetupRabbitMQ()

	// Initialize Elasticsearch and create the "plans" index
	config.SetupElasticsearch()
	if err := services.CreateIndexIfNotExists(indexName); err != nil {
		log.Fatalf("Failed to create Elasticsearch index: %v", err)
	}

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

	// Start RabbitMQ consumer in a separate goroutine
	go func() {
		err := services.ConsumeMessages(queueName, services.ProcessMessage)
		if err != nil {
			log.Fatalf("Error starting RabbitMQ consumer: %v", err)
		}
	}()

	// Start the server
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080" // Default port if not specified in .env
	}
	if err := r.Run(fmt.Sprintf(":%s", port)); err != nil {
		fmt.Printf("Failed to start server: %v\n", err)
	}
}
