package utils

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
)

func GenerateETag(data interface{}) string {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return ""
	}

	hash := md5.Sum(jsonData)
	return hex.EncodeToString(hash[:])
}
