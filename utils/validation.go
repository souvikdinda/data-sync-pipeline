package utils

import (
	"time"

	"github.com/go-playground/validator/v10"
)

var validate *validator.Validate

func init() {
	validate = validator.New()
	validate.RegisterValidation("copay", validateCopay)
	validate.RegisterValidation("date", validateDate)
}

func ValidateStruct(data interface{}) error {
	return validate.Struct(data)
}

func validateCopay(fl validator.FieldLevel) bool {
	copay := fl.Field().Int()
	return copay >= 0
}

func validateDate(fl validator.FieldLevel) bool {
	date := fl.Field().String()
	_, err := time.Parse("01-02-2006", date)
	return err == nil
}
