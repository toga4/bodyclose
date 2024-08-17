package a

import "net/http"

type foo struct{}

func (f *foo) struct_test1() {
	resp, err := http.Get("https://example.com")
	if err != nil {
		return
	}
	defer resp.Body.Close()
}

func (f *foo) struct_test2() {
	resp := f.getResp()
	defer resp.Body.Close()
}

func (f *foo) getResp() *http.Response {
	resp, err := http.Get("https://example.com")
	if err != nil {
		return nil
	}
	return resp
}

func struct_test10(f *foo) {
	resp := f.getResp()
	defer resp.Body.Close()
}
