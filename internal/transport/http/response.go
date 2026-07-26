package http

import "github.com/gofiber/fiber/v2"

// errorResponse
type errorResponse struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func respondError(c *fiber.Ctx, status int, code, message string) error {
	return c.Status(status).JSON(errorResponse{
		Error: errorBody{
			Code:    code,
			Message: message,
		},
	})
}
