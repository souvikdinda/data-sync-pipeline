package services

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"strings"

	"csye7255-project-one/config"
	"csye7255-project-one/models"

	"github.com/elastic/go-elasticsearch/v8/esapi"
)

func CreateIndexIfNotExists(indexName string) error {
	mapping := `
	{
		"mappings": {
			"properties": {
				"relation": {
					"type": "join",
					"relations": {
						"plan": ["planCostShares", "linkedPlanServices"],
						"planCostShares": [],
						"linkedPlanServices": ["linkedService", "planServiceCostShares"],
						"linkedService": [],
						"planServiceCostShares": []
					}
				},
				"planCostShares": {
					"type": "nested",
					"properties": {
						"_org": { "type": "text" },
						"copay": { "type": "integer" },
						"deductible": { "type": "integer" },
						"objectId": { "type": "text" },
						"objectType": { "type": "text" }
					}
				},
				"linkedPlanServices": {
					"type": "nested",
					"properties": {
						"_org": { "type": "text" },
						"objectId": { "type": "text" },
						"objectType": { "type": "text" },
						"linkedService": {
							"type": "nested",
							"properties": {
								"_org": { "type": "text" },
								"objectId": { "type": "text" },
								"objectType": { "type": "text" },
								"name": { "type": "text" }
							}
						},
						"planserviceCostShares": {
							"type": "nested",
							"properties": {
								"_org": { "type": "text" },
								"copay": { "type": "integer" },
								"deductible": { "type": "integer" },
								"objectId": { "type": "text" },
								"objectType": { "type": "text" }
							}
						}
					}
				},
				"_org": { "type": "text" },
				"objectId": { "type": "text" },
				"objectType": { "type": "text" },
				"planType": { "type": "text" },
				"creationDate": { "type": "text" }
			}
		}
	}`

	existsReq := esapi.IndicesExistsRequest{
		Index: []string{indexName},
	}

	existsRes, err := existsReq.Do(context.Background(), config.ESClient)
	if err != nil {
		return fmt.Errorf("error checking if index exists: %v", err)
	}
	defer existsRes.Body.Close()

	if existsRes.StatusCode == 200 {
		log.Printf("Index %s already exists", indexName)
		return nil
	} else if existsRes.StatusCode != 404 {
		return fmt.Errorf("unexpected status code when checking index existence: %d", existsRes.StatusCode)
	}

	createReq := esapi.IndicesCreateRequest{
		Index: indexName,
		Body:  strings.NewReader(mapping),
	}

	createRes, err := createReq.Do(context.Background(), config.ESClient)
	if err != nil {
		return fmt.Errorf("error creating index: %v", err)
	}
	defer createRes.Body.Close()

	if createRes.IsError() {
		return fmt.Errorf("error creating index: %s", createRes.String())
	}

	log.Printf("Index %s created successfully with the specified parent-child mapping.", indexName)
	return nil
}

func SaveParentAndChildrenToElasticsearch(index string, plan models.Plan) error {
	planDoc := map[string]interface{}{
		"relation": map[string]interface{}{
			"name": "plan",
		},
		"_org":         plan.Org,
		"objectId":     plan.ObjectId,
		"objectType":   plan.ObjectType,
		"planType":     plan.PlanType,
		"creationDate": plan.CreationDate,
	}
	if err := saveToElasticsearch(index, plan.ObjectId, planDoc, ""); err != nil {
		return fmt.Errorf("failed to save plan document: %v", err)
	}

	planCostSharesDoc := map[string]interface{}{
		"relation": map[string]interface{}{
			"name":   "planCostShares",
			"parent": plan.ObjectId,
		},
		"_org":       plan.PlanCostShares.Org,
		"copay":      plan.PlanCostShares.Copay,
		"deductible": plan.PlanCostShares.Deductible,
		"objectId":   plan.PlanCostShares.ObjectId,
		"objectType": plan.PlanCostShares.ObjectType,
	}
	if err := saveToElasticsearch(index, plan.PlanCostShares.ObjectId, planCostSharesDoc, plan.ObjectId); err != nil {
		return fmt.Errorf("failed to save PlanCostShares document: %v", err)
	}

	for _, linkedService := range plan.LinkedPlanServices {
		linkedPlanServiceDoc := map[string]interface{}{
			"relation": map[string]interface{}{
				"name":   "linkedPlanServices",
				"parent": plan.ObjectId,
			},
			"_org":       linkedService.Org,
			"objectId":   linkedService.ObjectId,
			"objectType": linkedService.ObjectType,
		}
		if err := saveToElasticsearch(index, linkedService.ObjectId, linkedPlanServiceDoc, plan.ObjectId); err != nil {
			return fmt.Errorf("failed to save LinkedPlanService document: %v", err)
		}

		linkedServiceDoc := map[string]interface{}{
			"relation": map[string]interface{}{
				"name":   "linkedService",
				"parent": linkedService.ObjectId,
			},
			"_org":       linkedService.LinkedService.Org,
			"objectId":   linkedService.LinkedService.ObjectId,
			"objectType": linkedService.LinkedService.ObjectType,
			"name":       linkedService.LinkedService.Name,
		}
		if err := saveToElasticsearch(index, linkedService.LinkedService.ObjectId, linkedServiceDoc, linkedService.ObjectId); err != nil {
			return fmt.Errorf("failed to save LinkedService document: %v", err)
		}

		planServiceCostSharesDoc := map[string]interface{}{
			"relation": map[string]interface{}{
				"name":   "planServiceCostShares",
				"parent": linkedService.ObjectId,
			},
			"_org":       linkedService.PlanServiceCostShares.Org,
			"copay":      linkedService.PlanServiceCostShares.Copay,
			"deductible": linkedService.PlanServiceCostShares.Deductible,
			"objectId":   linkedService.PlanServiceCostShares.ObjectId,
			"objectType": linkedService.PlanServiceCostShares.ObjectType,
		}
		if err := saveToElasticsearch(index, linkedService.PlanServiceCostShares.ObjectId, planServiceCostSharesDoc, linkedService.ObjectId); err != nil {
			return fmt.Errorf("failed to save PlanServiceCostShares document: %v", err)
		}
	}
	return nil
}

func PatchParentAndChildren(index string, plan models.Plan) error {
	planDoc := map[string]interface{}{
		"relation": map[string]interface{}{
			"name": "plan",
		},
		"_org":         plan.Org,
		"objectId":     plan.ObjectId,
		"objectType":   plan.ObjectType,
		"planType":     plan.PlanType,
		"creationDate": plan.CreationDate,
	}

	if err := saveOrUpdateChild(index, plan.ObjectId, planDoc, ""); err != nil {
		return fmt.Errorf("failed to update Plan document: %v", err)
	}

	if len(plan.PlanCostShares.ObjectId) > 0 {
		planCostSharesDoc := map[string]interface{}{
			"relation": map[string]interface{}{
				"name":   "planCostShares",
				"parent": plan.ObjectId,
			},
			"_org":       plan.PlanCostShares.Org,
			"copay":      plan.PlanCostShares.Copay,
			"deductible": plan.PlanCostShares.Deductible,
			"objectId":   plan.PlanCostShares.ObjectId,
			"objectType": plan.PlanCostShares.ObjectType,
		}
		if err := saveOrUpdateChild(index, plan.PlanCostShares.ObjectId, planCostSharesDoc, plan.ObjectId); err != nil {
			return fmt.Errorf("failed to update PlanCostShares document: %v", err)
		}
	}

	for _, linkedPlanService := range plan.LinkedPlanServices {
		linkedPlanServiceDoc := map[string]interface{}{
			"relation": map[string]interface{}{
				"name":   "linkedPlanServices",
				"parent": plan.ObjectId,
			},
			"_org":       linkedPlanService.Org,
			"objectId":   linkedPlanService.ObjectId,
			"objectType": linkedPlanService.ObjectType,
		}
		if err := saveOrUpdateChild(index, linkedPlanService.ObjectId, linkedPlanServiceDoc, plan.ObjectId); err != nil {
			return fmt.Errorf("failed to update LinkedPlanService document: %v", err)
		}

		linkedServiceDoc := map[string]interface{}{
			"relation": map[string]interface{}{
				"name":   "linkedService",
				"parent": linkedPlanService.ObjectId,
			},
			"_org":       linkedPlanService.LinkedService.Org,
			"objectId":   linkedPlanService.LinkedService.ObjectId,
			"objectType": linkedPlanService.LinkedService.ObjectType,
			"name":       linkedPlanService.LinkedService.Name,
		}
		if err := saveOrUpdateChild(index, linkedPlanService.LinkedService.ObjectId, linkedServiceDoc, linkedPlanService.ObjectId); err != nil {
			return fmt.Errorf("failed to update LinkedService document: %v", err)
		}

		planServiceCostSharesDoc := map[string]interface{}{
			"relation": map[string]interface{}{
				"name":   "planServiceCostShares",
				"parent": linkedPlanService.ObjectId,
			},
			"_org":       linkedPlanService.PlanServiceCostShares.Org,
			"copay":      linkedPlanService.PlanServiceCostShares.Copay,
			"deductible": linkedPlanService.PlanServiceCostShares.Deductible,
			"objectId":   linkedPlanService.PlanServiceCostShares.ObjectId,
			"objectType": linkedPlanService.PlanServiceCostShares.ObjectType,
		}
		if err := saveOrUpdateChild(index, linkedPlanService.PlanServiceCostShares.ObjectId, planServiceCostSharesDoc, linkedPlanService.ObjectId); err != nil {
			return fmt.Errorf("failed to update PlanServiceCostShares document: %v", err)
		}
	}

	return nil
}

func DeleteParentAndChildren(index string, parentID string) error {
	if err := deleteDescendants(index, parentID, "planCostShares"); err != nil {
		return fmt.Errorf("failed to delete descendants of PlanCostShares: %v", err)
	}
	if err := deleteDescendants(index, parentID, "linkedPlanServices"); err != nil {
		return fmt.Errorf("failed to delete descendants of LinkedPlanServices: %v", err)
	}

	if err := deleteFromElasticsearch(index, parentID); err != nil {
		return fmt.Errorf("failed to delete Plan document: %v", err)
	}

	log.Printf("Successfully deleted Plan document %s and all its descendants", parentID)
	return nil
}

func deleteDescendants(index string, parentID string, childType string) error {
	searchQuery := map[string]interface{}{
		"query": map[string]interface{}{
			"parent_id": map[string]interface{}{
				"type": childType,
				"id":   parentID,
			},
		},
	}
	searchBody, err := json.Marshal(searchQuery)
	if err != nil {
		return fmt.Errorf("failed to create search query for %s: %v", childType, err)
	}

	searchReq := esapi.SearchRequest{
		Index: []string{index},
		Body:  bytes.NewReader(searchBody),
	}
	searchRes, err := searchReq.Do(context.Background(), config.ESClient)
	if err != nil {
		return fmt.Errorf("failed to search for %s children: %v", childType, err)
	}
	defer searchRes.Body.Close()

	if searchRes.IsError() {
		return fmt.Errorf("failed to search for %s children: %s", childType, searchRes.Status())
	}

	var searchResults struct {
		Hits struct {
			Hits []struct {
				ID string `json:"_id"`
			} `json:"hits"`
		} `json:"hits"`
	}
	if err := json.NewDecoder(searchRes.Body).Decode(&searchResults); err != nil {
		return fmt.Errorf("failed to parse search results for %s: %v", childType, err)
	}

	for _, hit := range searchResults.Hits.Hits {
		childID := hit.ID

		if childType == "linkedPlanServices" {
			if err := deleteDescendants(index, childID, "linkedService"); err != nil {
				return fmt.Errorf("failed to delete linkedService of %s: %v", childID, err)
			}
			if err := deleteDescendants(index, childID, "planServiceCostShares"); err != nil {
				return fmt.Errorf("failed to delete planServiceCostShares of %s: %v", childID, err)
			}
		}

		if err := deleteFromElasticsearch(index, childID); err != nil {
			return fmt.Errorf("failed to delete child document %s: %v", childID, err)
		}
	}

	return nil
}

func saveToElasticsearch(index, docID string, data interface{}, parentID string) error {
	if config.ESClient == nil {
		return errors.New("elasticsearch client is not initialized")
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal data: %v", err)
	}

	req := esapi.IndexRequest{
		Index:      index,
		DocumentID: docID,
		Body:       bytes.NewReader(jsonData),
		Routing:    parentID,
		Refresh:    "true",
	}

	res, err := req.Do(context.Background(), config.ESClient)
	if err != nil {
		return fmt.Errorf("failed to index document: %v", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		body, _ := io.ReadAll(res.Body)
		return fmt.Errorf("failed to save document to Elasticsearch: %s, response: %s", res.Status(), string(body))
	}

	log.Printf("Document saved to Elasticsearch index: %s, ID: %s", index, docID)
	return nil
}

func updateInElasticsearch(index, docID string, data interface{}, parentID string) error {
	if config.ESClient == nil {
		return errors.New("elasticsearch client is not initialized")
	}

	jsonData, err := json.Marshal(map[string]interface{}{"doc": data})
	if err != nil {
		return fmt.Errorf("failed to marshal data: %v", err)
	}

	req := esapi.UpdateRequest{
		Index:      index,
		DocumentID: docID,
		Body:       bytes.NewReader(jsonData),
		Routing:    parentID,
		Refresh:    "true",
	}

	res, err := req.Do(context.Background(), config.ESClient)
	if err != nil {
		return fmt.Errorf("failed to update document: %v", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return fmt.Errorf("failed to update document in Elasticsearch: %s", res.Status())
	}

	log.Printf("Document updated in Elasticsearch index: %s, ID: %s", index, docID)
	return nil
}

func saveOrUpdateChild(index, docID string, data interface{}, parentID string) error {
	exists, err := documentExists(index, docID)
	if err != nil {
		return fmt.Errorf("failed to check document existence: %v", err)
	}

	if exists {
		return updateInElasticsearch(index, docID, data, parentID)
	}

	return saveToElasticsearch(index, docID, data, parentID)
}

func documentExists(index, docID string) (bool, error) {
	req := esapi.GetRequest{
		Index:      index,
		DocumentID: docID,
	}

	res, err := req.Do(context.Background(), config.ESClient)
	if err != nil {
		return false, fmt.Errorf("error checking document existence: %v", err)
	}
	defer res.Body.Close()

	return res.StatusCode == 200, nil
}

func deleteFromElasticsearch(index, docID string) error {
	req := esapi.DeleteRequest{
		Index:      index,
		DocumentID: docID,
	}

	res, err := req.Do(context.Background(), config.ESClient)
	if err != nil {
		return fmt.Errorf("error deleting document: %v", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return fmt.Errorf("failed to delete document from Elasticsearch: %s", res.Status())
	}

	log.Printf("Document deleted from Elasticsearch index: %s, ID: %s", index, docID)
	return nil
}
