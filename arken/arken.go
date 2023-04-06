package arken

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/pkg/errors"
	"github.com/valyala/fasthttp"
)

// Client Arken Publick API Client
type Client struct {
	url         url.URL
	apiToken    string
	apiUserName string
}

// New Arken Publick API Client
func New(apiURL, apiUserName, apiToken string) *Client {
	if apiURL == "" {
		apiURL = "https://public-api.arken.finance"
	}

	return &Client{
		url:         *Must(url.Parse(apiURL)),
		apiToken:    apiToken,
		apiUserName: apiUserName,
	}
}

// doRequest do request to Arken Publick API and unmarshal response body to v
func (c *Client) doRequest(ctx context.Context, path string, v any) (status int, err error) {
	url := c.url.JoinPath(path).String()

	req := fasthttp.AcquireRequest()
	req.Header.SetMethod(http.MethodGet)
	req.SetRequestURI(url)
	req.Header.Set("X-API-Username", c.apiUserName)
	req.Header.Set("X-API-Token", c.apiToken)

	resp := fasthttp.AcquireResponse()
	if err := fasthttp.Do(req, resp); err != nil {
		return 0, errors.Wrapf(err, "url: %s", url)
	}
	fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	statusCode := resp.Header.StatusCode()
	if statusCode != http.StatusOK {
		return statusCode, errors.Errorf("status code: %d, url: %s", statusCode, url)
	}

	respBody := resp.Body()
	if err := json.Unmarshal(respBody, v); err != nil {
		return 0, errors.Wrapf(err, "cannot unmarshal response body: %s", string(respBody))
	}

	return statusCode, nil
}
