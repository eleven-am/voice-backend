package dto

type ErrorResponse struct {
	Code    string `json:"code" example:"invalid_request"`
	Message string `json:"message" example:"Invalid request body"`
	Details any    `json:"details,omitempty" swaggertype:"object"`
}

type ValidationError struct {
	Field   string `json:"field" example:"email"`
	Message string `json:"message" example:"Email is required"`
}
