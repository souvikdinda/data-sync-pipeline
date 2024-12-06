package services

import (
	"context"
	"csye7255-project-one/config"
	"encoding/json"

	"github.com/redis/go-redis/v9"
)

func CheckIfRecordExists(id string) (bool, error) {
	exists, err := config.RedisClient.HExists(context.Background(), "plans", id).Result()
	if err != nil {
		return false, err
	}
	return exists, nil
}

func SaveRecord(id string, data interface{}) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	return config.RedisClient.HSet(context.Background(), "plans", id, jsonData).Err()
}

func GetRecord(id string) (map[string]interface{}, error) {
	result, err := config.RedisClient.HGet(context.Background(), "plans", id).Result()
	if err == redis.Nil {
		return nil, nil
	} else if err != nil {
		return nil, err
	}

	var record map[string]interface{}
	if err := json.Unmarshal([]byte(result), &record); err != nil {
		return nil, err
	}

	return record, nil
}

func GetAllRecords() ([]map[string]interface{}, error) {
	results, err := config.RedisClient.HGetAll(context.Background(), "plans").Result()
	if err != nil {
		return nil, err
	}

	var records []map[string]interface{}
	for _, jsonString := range results {
		var record map[string]interface{}
		if err := json.Unmarshal([]byte(jsonString), &record); err != nil {
			continue
		}
		records = append(records, record)
	}
	return records, nil
}

func DeleteRecord(id string) error {
	return config.RedisClient.HDel(context.Background(), "plans", id).Err()
}
