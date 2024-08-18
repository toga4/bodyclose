package a

import (
	"io"
	"net/http"
)

func func_test1() {
	resp, err := http.Get("https://example.com")
	if err != nil {
		return
	}
	defer resp.Body.Close()
}

func func_test2() {
	resp := getResp()
	defer resp.Body.Close()
}

func getResp() *http.Response {
	resp, err := http.Get("https://example.com")
	if err != nil {
		return nil
	}
	return resp
}

func func_test3() {
	resp, err := http.Get("https://example.com")
	if err != nil {
		return
	}
	defer func() {
		resp.Body.Close()
	}()
}

func func_test4() {
	resp, err := http.Get("https://example.com")
	if err != nil {
		return
	}
	body := resp.Body
	defer func() {
		body.Close()
	}()
}

func func_test5() {
	b := getBody()
	defer b.Close()
}

func getBody() io.ReadCloser {
	resp, err := http.Get("https://example.com")
	if err != nil {
		return nil
	}
	return resp.Body
}

func func_test6() {
	resp := getClosedResp()
	_ = resp
}

func getClosedResp() *http.Response {
	resp, err := http.Get("https://example.com")
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	return resp
}

func func_test7() {
	resp, err := http.Get("https://example.com")
	if err != nil {
		return
	}
	resp.Cookies()
	closeResp(resp)
}

func closeResp(resp *http.Response) {
	defer resp.Body.Close()
}

func func_test8() {
	resp, err := http.Get("https://example.com")
	if err != nil {
		return
	}
	resp.Body.Read(make([]byte, 0))
	closeBody(resp.Body)
}

func closeBody(b io.Closer) {
	defer b.Close()
}

func func_test9() {
	resp, err := http.Get("https://example.com")
	if err != nil {
		return
	}
	resp.Body.Read(make([]byte, 0))
	defer closeBody(resp.Body)
}
