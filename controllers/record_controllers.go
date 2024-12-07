package controllers

import (
	"csye7255-project-one/models"
	"csye7255-project-one/services"
	"csye7255-project-one/utils"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

var (
	index     = "plans"
	queueName = "plan_requests"
)

func CreateRecord(c *gin.Context) {
	var plan models.Plan

	if err := c.ShouldBindJSON(&plan); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := utils.ValidateStruct(plan); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	exists, err := services.CheckIfRecordExists(plan.ObjectId)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check existence of the record"})
		return
	}
	if exists {
		c.Status(http.StatusConflict)
		return
	}

	err = services.SaveRecord(plan.ObjectId, plan)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save data to Redis"})
		return
	}

	savedRecord, err := services.GetRecord(plan.ObjectId)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch saved data from Redis"})
		return
	}

	if err := PublishOperationToQueue("POST", index, plan.ObjectId, plan); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to publish message to RabbitMQ"})
		return
	}

	savedRecordJSON, err := json.Marshal(savedRecord)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to convert record to JSON"})
		return
	}

	etag := utils.GenerateETag(savedRecordJSON)
	c.Header("ETag", etag)
	c.JSON(http.StatusCreated, savedRecord)
}

func GetRecord(c *gin.Context) {
	id := c.Param("id")
	record, err := services.GetRecord(id)
	if err == redis.Nil || record == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Record not found"})
		return
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch data from Redis"})
		return
	}
	recordJSON, err := json.Marshal(record)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to convert record to JSON"})
		return
	}

	c.Header("Cache-Control", "no-store, no-cache, must-revalidate")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Keep-Alive", "timeout=5, max=60")
	c.Header("Connection", "Keep-Alive")
	c.Header("Content-Type", "application/json")

	etag := utils.GenerateETag(recordJSON)
	c.Header("ETag", etag)

	if match := c.GetHeader("If-None-Match"); match == etag {
		c.Status(http.StatusNotModified)
		return
	}

	c.Data(http.StatusOK, "application/json", recordJSON)
}

func PatchRecord(c *gin.Context) {
	id := c.Param("id")
	existingRecord, err := services.GetRecord(id)
	if err == redis.Nil || existingRecord == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Record not found"})
		return
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch data from Redis"})
		return
	}

	existingRecordJSON, err := json.Marshal(existingRecord)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to serialize existing record"})
		return
	}

	var plan models.Plan
	if err := json.Unmarshal(existingRecordJSON, &plan); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse existing record"})
		return
	}

	existingETag := utils.GenerateETag(existingRecordJSON)
	clientIfMatch := c.GetHeader("If-Match")
	clientIfNoneMatch := c.GetHeader("If-None-Match")

	if clientIfMatch == "" && clientIfNoneMatch == "" {
		c.JSON(http.StatusPreconditionRequired, gin.H{"error": "At least one of If-Match or If-None-Match headers is required"})
		return
	}

	if clientIfMatch != "" && clientIfMatch != existingETag {
		c.JSON(http.StatusPreconditionFailed, gin.H{"error": "ETag mismatch. The resource has been modified by another process."})
		return
	}

	if clientIfNoneMatch == existingETag {
		c.JSON(http.StatusNotModified, gin.H{"message": "Resource not modified"})
		return
	}

	var updates map[string]interface{}
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON data"})
		return
	}

	if newLinkedServices, ok := updates["linkedPlanServices"].([]interface{}); ok {
		for _, ls := range newLinkedServices {
			var newService models.LinkedPlanService
			lsBytes, _ := json.Marshal(ls)
			if err := json.Unmarshal(lsBytes, &newService); err == nil {
				exists := false
				for i, existingService := range plan.LinkedPlanServices {
					if existingService.ObjectId == newService.ObjectId {
						plan.LinkedPlanServices[i] = newService
						exists = true
						break
					}
				}
				if !exists {
					plan.LinkedPlanServices = append(plan.LinkedPlanServices, newService)
				}
			}
		}
	}

	delete(updates, "linkedPlanServices")

	updatesJSON, err := json.Marshal(updates)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to encode updates"})
		return
	}
	if err := json.Unmarshal(updatesJSON, &plan); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to apply updates"})
		return
	}

	if err := utils.ValidateStruct(plan); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := services.SaveRecord(id, plan); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update data"})
		return
	}

	if err := PublishOperationToQueue("PATCH", index, id, plan); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to publish operation to RabbitMQ"})
		return
	}

	savedRecord, err := services.GetRecord(plan.ObjectId)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch saved data from Redis"})
		return
	}
	savedRecordJSON, err := json.Marshal(savedRecord)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to convert updated record to JSON"})
		return
	}
	etag := utils.GenerateETag(savedRecordJSON)
	c.Header("ETag", etag)
	c.JSON(http.StatusOK, plan)
}

func PutRecord(c *gin.Context) {
	id := c.Param("id")

	var newRecord models.Plan
	if err := c.ShouldBindJSON(&newRecord); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON data"})
		return
	}

	if err := utils.ValidateStruct(newRecord); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	existingRecord, err := services.GetRecord(id)
	if err == redis.Nil || existingRecord == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Record not found"})
		return
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch data from Redis"})
		return
	}

	var existingRecordJSON []byte
	var existingETag string

	existingRecordJSON, err = json.Marshal(existingRecord)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to serialize existing record"})
		return
	}
	existingETag = utils.GenerateETag(existingRecordJSON)

	clientIfMatch := c.GetHeader("If-Match")
	clientIfNoneMatch := c.GetHeader("If-None-Match")

	if clientIfMatch == "" && clientIfNoneMatch == "" {
		c.JSON(http.StatusPreconditionRequired, gin.H{"error": "At least one of If-Match or If-None-Match headers is required"})
		return
	}

	if clientIfMatch != "" && clientIfMatch != existingETag {
		c.JSON(http.StatusPreconditionFailed, gin.H{"error": "ETag mismatch. The resource has been modified by another process."})
		return
	}

	if clientIfNoneMatch != "" && clientIfNoneMatch == existingETag {
		c.JSON(http.StatusPreconditionFailed, gin.H{"error": "Resource already exists"})
		return
	}

	if err := services.SaveRecord(id, newRecord); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save data to Redis"})
		return
	}

	if err := PublishOperationToQueue("PUT", index, id, newRecord); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to publish operation to RabbitMQ"})
		return
	}

	savedRecord, err := services.GetRecord(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch saved data from Redis"})
		return
	}
	savedRecordJSON, err := json.Marshal(savedRecord)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to convert updated record to JSON"})
		return
	}
	etag := utils.GenerateETag(savedRecordJSON)
	c.Header("ETag", etag)
	c.JSON(http.StatusOK, newRecord)
}

func DeleteRecord(c *gin.Context) {
	id := c.Param("id")

	existingRecord, err := services.GetRecord(id)
	if err == redis.Nil || existingRecord == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Record not found"})
		return
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch data from Redis"})
		return
	}

	existingRecordJSON, err := json.Marshal(existingRecord)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to serialize existing record"})
		return
	}

	var plan models.Plan
	if err := json.Unmarshal(existingRecordJSON, &plan); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse existing record"})
		return
	}

	existingETag := utils.GenerateETag(existingRecordJSON)
	clientIfMatch := c.GetHeader("If-Match")

	if clientIfMatch == "" {
		c.JSON(http.StatusPreconditionRequired, gin.H{"error": "If-Match header is required"})
		return
	}

	if clientIfMatch != "" && clientIfMatch != existingETag {
		c.JSON(http.StatusPreconditionFailed, gin.H{"error": "ETag mismatch. The resource has been modified by another process."})
		return
	}

	err = services.DeleteRecord(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete data from Redis"})
		return
	}

	if err := PublishOperationToQueue("DELETE", index, id, nil); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to publish delete operation to RabbitMQ"})
		return
	}

	c.Status(http.StatusNoContent)
}

func PublishOperationToQueue(operation, index, docID string, payload interface{}) error {
	message := map[string]interface{}{
		"operation": operation,
		"index":     index,
		"doc_id":    docID,
		"payload":   payload,
	}
	messageJSON, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to serialize message: %v", err)
	}

	return services.PublishMessage(queueName, messageJSON)
}
