package e2e_tests

import (
	"io"
	"net/http"
)

func httpDelete(url string, body io.Reader) (response *http.Response, err error) {
	req, _ := http.NewRequest(http.MethodDelete, url, body)
	client := &http.Client{}
	return client.Do(req)
}
