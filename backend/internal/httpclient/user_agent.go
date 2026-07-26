package httpclient

import "net/http"

const BrowserUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/136.0.0.0 Safari/537.36"

func SetBrowserUserAgent(request *http.Request) {
	if request != nil {
		request.Header.Set("User-Agent", BrowserUserAgent)
	}
}
