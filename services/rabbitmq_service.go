package services

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"

	"csye7255-project-one/config"
	"csye7255-project-one/models"

	amqp "github.com/rabbitmq/amqp091-go"
)

func PublishMessage(queueName string, message []byte) error {
	ch, err := config.GetRabbitMQChannel()
	if err != nil {
		log.Printf("Failed to get RabbitMQ channel: %v", err)
		return err
	}
	defer ch.Close()

	q, err := ch.QueueDeclare(
		queueName,
		false, // durable
		false, // auto-delete
		false, // exclusive
		false, // no-wait
		nil,   // arguments
	)
	if err != nil {
		log.Printf("Failed to declare RabbitMQ queue: %v", err)
		return err
	}

	// Publish the message
	err = ch.Publish(
		"",     // exchange
		q.Name, // routing key
		false,  // mandatory
		false,  // immediate
		amqp.Publishing{
			ContentType: "application/json",
			Body:        message,
		},
	)
	if err != nil {
		log.Printf("Failed to publish message to RabbitMQ: %v", err)
		return err
	}

	log.Printf("Message successfully published to RabbitMQ queue: %s", queueName)
	return nil
}

func ConsumeMessages(queueName string, handler func([]byte) error) error {
	ch, err := config.GetRabbitMQChannel()
	if err != nil {
		log.Printf("Failed to get RabbitMQ channel: %v", err)
		return err
	}
	defer ch.Close()

	q, err := ch.QueueDeclare(
		queueName,
		false, // durable
		false, // auto-delete
		false, // exclusive
		false, // no-wait
		nil,   // arguments
	)
	if err != nil {
		log.Printf("Failed to declare RabbitMQ queue: %v", err)
		return err
	}

	msgs, err := ch.Consume(
		q.Name,
		"",    // consumer tag
		true,  // auto-ack
		false, // exclusive
		false, // no-local
		false, // no-wait
		nil,   // args
	)
	if err != nil {
		log.Printf("Failed to register RabbitMQ consumer: %v", err)
		return err
	}

	go func() {
		for d := range msgs {
			log.Printf("Received message from RabbitMQ queue: %s", queueName)

			if err := handler(d.Body); err != nil {
				log.Printf("Failed to process message: %v", err)
			}
		}
	}()

	log.Printf("Listening for messages on RabbitMQ queue: %s", queueName)
	select {} // Block forever
}

func ProcessMessage(message []byte) error {
	var msg map[string]interface{}
	if err := json.Unmarshal(message, &msg); err != nil {
		return errors.New("failed to deserialize message: " + err.Error())
	}

	operation := msg["operation"].(string)
	index := msg["index"].(string)
	docID := msg["doc_id"].(string)
	var payload map[string]interface{}
	if msgPayload, ok := msg["payload"]; ok && msgPayload != nil {
		payload, _ = msgPayload.(map[string]interface{})
	}

	switch operation {
	case "POST":
		plan := models.Plan{}
		if err := mapToStruct(payload, &plan); err != nil {
			return fmt.Errorf("failed to map payload to Plan struct: %v", err)
		}
		if err := SaveParentAndChildrenToElasticsearch(index, plan); err != nil {
			return fmt.Errorf("failed to save parent and children to Elasticsearch: %v", err)
		}
	case "PUT":
		plan := models.Plan{}
		if err := mapToStruct(payload, &plan); err != nil {
			return fmt.Errorf("failed to map payload to Plan struct: %v", err)
		}
		if err := SaveParentAndChildrenToElasticsearch(index, plan); err != nil {
			return fmt.Errorf("failed to update parent and children in Elasticsearch: %v", err)
		}
	case "PATCH":
		plan := models.Plan{}
		if err := mapToStruct(payload, &plan); err != nil {
			return fmt.Errorf("failed to map payload to Plan struct: %v", err)
		}
		if err := PatchParentAndChildren(index, plan); err != nil {
			return fmt.Errorf("failed to patch parent and children in Elasticsearch: %v", err)
		}
	case "DELETE":
		if err := DeleteParentAndChildren(index, docID); err != nil {
			return fmt.Errorf("failed to delete parent and children from Elasticsearch: %v", err)
		}
	default:
		return fmt.Errorf("unknown operation: %s", operation)
	}

	log.Printf("Successfully processed %s operation for document ID: %s", operation, docID)
	return nil
}

// mapToStruct maps a generic map to a specific struct
func mapToStruct(data map[string]interface{}, obj interface{}) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return json.Unmarshal(jsonData, obj)
}
