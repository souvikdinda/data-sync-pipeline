package models

type LinkedService struct {
	Org        string `json:"_org" validate:"required"`
	ObjectId   string `json:"objectId" validate:"required"`
	ObjectType string `json:"objectType" validate:"required"`
	Name       string `json:"name" validate:"required"`
}

type PlanServiceCostShares struct {
	Deductible int    `json:"deductible" validate:"required"`
	Org        string `json:"_org" validate:"required"`
	Copay      int    `json:"copay" validate:"copay"`
	ObjectId   string `json:"objectId" validate:"required"`
	ObjectType string `json:"objectType" validate:"required"`
}

type LinkedPlanService struct {
	LinkedService         LinkedService         `json:"linkedService" validate:"required"`
	PlanServiceCostShares PlanServiceCostShares `json:"planserviceCostShares" validate:"required"`
	Org                   string                `json:"_org" validate:"required"`
	ObjectId              string                `json:"objectId" validate:"required"`
	ObjectType            string                `json:"objectType" validate:"required"`
}

type PlanCostShares struct {
	Deductible int    `json:"deductible" validate:"required"`
	Org        string `json:"_org" validate:"required"`
	Copay      int    `json:"copay" validate:"required"`
	ObjectId   string `json:"objectId" validate:"required"`
	ObjectType string `json:"objectType" validate:"required"`
}

type Plan struct {
	PlanCostShares     PlanCostShares      `json:"planCostShares" validate:"required"`
	LinkedPlanServices []LinkedPlanService `json:"linkedPlanServices" validate:"required,dive"`
	Org                string              `json:"_org" validate:"required"`
	ObjectId           string              `json:"objectId" validate:"required"`
	ObjectType         string              `json:"objectType" validate:"required"`
	PlanType           string              `json:"planType" validate:"required"`
	CreationDate       string              `json:"creationDate" validate:"required,date"`
}
