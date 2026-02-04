package controller

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"runtime/debug"

	"github.com/IntegratedBrainEnvironment/BrainNav_GO_BE/helpers/utils"
)

type BaseController struct {
	logger *utils.Logger
}

func (c *BaseController) RespondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")

	defer func() {
		if r := recover(); r != nil {
			log.Printf("Panic in RespondWithJSON: %v\nStack: %s", r, debug.Stack())
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error": "Internal server error while creating response"}`))
		}
	}()

	response, err := json.Marshal(payload)
	if err != nil {
		log.Printf("Error marshalling JSON response: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(fmt.Sprintf(`{"error": "Failed to marshal JSON response: %v"}`, err)))
		return
	}

	w.WriteHeader(code)
	if _, err := w.Write(response); err != nil {
		log.Printf("Error writing response: %v", err)
	} else {
		log.Printf("Successfully wrote response with status code: %d", code)
	}
}

func (c *BaseController) RespondWithError(w http.ResponseWriter, err error) {
	var statusCode int
	var message string
	var code string
	var details map[string]interface{}

	var appErr *utils.AppError
	if utils.AsAppError(err, &appErr) {
		statusCode = appErr.StatusCode
		message = appErr.Message
		code = appErr.Code
		details = appErr.Details
		log.Printf("AppError: %s (Status: %d, Code: %s)", message, statusCode, code)
	} else {
		statusCode = http.StatusInternalServerError
		message = "Internal server error"
		log.Printf("Unhandled error: %v", err)
	}

	response := map[string]interface{}{"error": message}

	if code != "" {
		response["code"] = code
	}

	if details != nil && len(details) > 0 {
		response["details"] = details
	}

	c.RespondWithJSON(w, statusCode, response)
}
