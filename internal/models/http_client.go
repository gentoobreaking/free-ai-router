package models

import (
	"net/http"
	"time"
)

var httpClient = &http.Client{Timeout: 30 * time.Second}
