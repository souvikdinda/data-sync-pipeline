package config

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"os"
	"sync"

	"github.com/elastic/go-elasticsearch/v8"
	"github.com/golang-jwt/jwt/v5"
	"github.com/joho/godotenv"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/redis/go-redis/v9"
)

var (
	RedisClient  *redis.Client
	Ctx          = context.Background()
	RabbitMQConn *amqp.Connection
	ESClient     *elasticsearch.Client

	googleCertsURL = "https://www.googleapis.com/oauth2/v3/certs"
	googleCerts    map[string]*rsa.PublicKey
	certsMutex     sync.RWMutex
)

type GoogleCertsResponse struct {
	Keys []struct {
		Kid string `json:"kid"`
		N   string `json:"n"`
		E   string `json:"e"`
	} `json:"keys"`
}

func SetupRedis() {
	err := godotenv.Load()
	if err != nil {
		log.Fatalf("Error loading .env file")
	}

	RedisClient = redis.NewClient(&redis.Options{
		Addr: os.Getenv("REDIS_URL"),
	})

	_, err = RedisClient.Ping(Ctx).Result()
	if err != nil {
		fmt.Printf("Failed to connect to Redis: %v\n", err)
		panic(err)
	} else {
		fmt.Println("Connected to Redis successfully!")
	}
}

func SetupRabbitMQ() {
	var err error
	rabbitMQURL := fmt.Sprintf("amqp://%s:%s@%s:%s/", os.Getenv("RABBITMQ_USER"), os.Getenv("RABBITMQ_PASSWORD"), os.Getenv("RABBITMQ_HOST"), os.Getenv("RABBITMQ_PORT"))
	RabbitMQConn, err = amqp.Dial(rabbitMQURL)
	if err != nil {
		fmt.Printf("Failed to connect to RabbitMQ: %v\n", err)
		panic(err)
	} else {
		fmt.Println("Connected to RabbitMQ successfully!")
	}
}

func GetRabbitMQChannel() (*amqp.Channel, error) {
	if RabbitMQConn == nil {
		return nil, errors.New("RabbitMQ connection is not initialized")
	}
	return RabbitMQConn.Channel()
}

func SetupElasticsearch() {
	cfg := elasticsearch.Config{
		Addresses: []string{
			os.Getenv("ELASTICSEARCH_URL"),
		},
	}

	client, err := elasticsearch.NewClient(cfg)
	if err != nil {
		log.Fatalf("Error creating Elasticsearch client: %s", err)
	}
	ESClient = client
	fmt.Println("Connected to Elasticsearch successfully!")
}

func FetchGoogleCerts() error {
	resp, err := http.Get(googleCertsURL)
	if err != nil {
		return fmt.Errorf("failed to fetch Google certs: %v", err)
	}
	defer resp.Body.Close()

	var certsResponse GoogleCertsResponse
	if err := json.NewDecoder(resp.Body).Decode(&certsResponse); err != nil {
		return fmt.Errorf("failed to decode certs: %v", err)
	}

	certsMutex.Lock()
	defer certsMutex.Unlock()
	googleCerts = make(map[string]*rsa.PublicKey)

	for _, key := range certsResponse.Keys {
		modulus, err := base64.RawURLEncoding.DecodeString(key.N)
		if err != nil {
			return err
		}
		exponent, err := base64.RawURLEncoding.DecodeString(key.E)
		if err != nil {
			return err
		}
		n := new(big.Int)
		n.SetBytes(modulus)
		var e int
		if len(exponent) < 4 {
			for _, b := range exponent {
				e = e<<8 + int(b)
			}
		} else {
			return errors.New("exponent too large")
		}
		rsaPublicKey := &rsa.PublicKey{
			N: n,
			E: e,
		}
		googleCerts[key.Kid] = rsaPublicKey
	}

	return nil
}

func GetGoogleCert(kid string) (*rsa.PublicKey, error) {
	certsMutex.RLock()
	defer certsMutex.RUnlock()

	cert, exists := googleCerts[kid]
	if !exists {
		return nil, fmt.Errorf("no cert found for key ID: %s", kid)
	}
	return cert, nil
}

func ValidateGoogleToken(tokenString string) (*jwt.Token, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}

		kid, ok := token.Header["kid"].(string)
		if !ok {
			return nil, fmt.Errorf("kid header not found in JWT")
		}

		cert, err := GetGoogleCert(kid)
		if err != nil {
			return nil, err
		}

		return cert, nil
	})

	if err != nil {
		fmt.Printf("Token parsing error: %v\n", err)
		return nil, err
	}

	if claims, ok := token.Claims.(jwt.MapClaims); ok {
		fmt.Printf("Token claims: %v\n", claims)
	}

	return token, nil
}

func ValidateTokenAudience(token *jwt.Token) error {
	err := godotenv.Load()
	if err != nil {
		log.Fatalf("Error loading .env file")
	}
	clientID := os.Getenv("GOOGLE_CLIENT_ID")

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return errors.New("unable to parse claims")
	}

	if aud, ok := claims["aud"].(string); !ok || aud != clientID {
		return fmt.Errorf("invalid audience: expected %s but got %v", clientID, aud)
	}

	return nil
}

func InitGoogleCerts() {
	if err := FetchGoogleCerts(); err != nil {
		fmt.Printf("Error initializing Google certs: %v\n", err)
	}
}
