package a

import "net/http"

func func_error_test1() {
	resp, _ := http.Get("https://example.com") // want "response body must be closed"
	_ = resp
}
