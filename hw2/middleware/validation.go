package middleware

import (
	"errors"
	"hw2/internal/api"
	"net/http"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers/gorillamux"
	"github.com/gin-gonic/gin"
	ginmiddleware "github.com/oapi-codegen/gin-middleware"
)

func OpenAPIValidation() gin.HandlerFunc {
	spec, err := api.GetSpec()
	if err != nil {
		panic(err)
	}
	spec.Servers = nil
	r, err := gorillamux.NewRouter(spec)
	if err != nil {
		panic(err)
	}
	options := &ginmiddleware.Options{
		Options: openapi3filter.Options{
			MultiError: true,
		},
		MultiErrorHandler: func(me openapi3.MultiError) error {
			return me
		},
	}
	return func(c *gin.Context) {
		err := ginmiddleware.ValidateRequestFromContext(
			c, r, options,
		)
		if err == nil {
			c.Next()
			return
		}
		details := validationDetails(err)
		c.AbortWithStatusJSON(
			http.StatusBadRequest,
			api.ErrorResponse{
				ErrorCode: "VALIDATION_ERROR",
				Message:   "Invalid request",
				Details:   &details,
			},
		)
	}
}

func validationDetails(err error) map[string]interface{} {
	issues := []error{err}
	var multi openapi3.MultiError
	if errors.As(err, &multi) {
		issues = multi
	}
	violations := make([]map[string]string, 0, len(issues))
	for _, issue := range issues {
		field := "request"
		message := issue.Error()
		var requestErr *openapi3filter.RequestError
		if errors.As(issue, &requestErr) && requestErr.Parameter != nil {
			field = requestErr.Parameter.Name
		}
		var schemaErr *openapi3.SchemaError
		if errors.As(issue, &schemaErr) {
			message = schemaErr.Reason
			if path := schemaErr.JSONPointer(); len(path) > 0 {
				field = strings.Join(path, ".")
			}
		}
		violations = append(violations, map[string]string{
			"field":   field,
			"message": message,
		})
	}
	return map[string]interface{}{
		"violations": violations,
	}
}
