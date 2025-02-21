package coingecko

import (
	"context"
	"net/http"
	"net/url"
	"path"

	"github.com/cockroachdb/errors"
	"github.com/gaze-network/indexer-network/common/errs"
	"github.com/gaze-network/indexer-network/pkg/httpclient"
	"github.com/valyala/fasthttp"
)

type coinHistoricalPriceResponse struct {
	Prices [][]float64 `json:"prices"`
	// MarketCaps   [][]float64 `json:"market_caps"`
	// TotalVolumes [][]float64 `json:"total_volumes"`
}

func (c *Client) GetPrice(ctx context.Context, chainId string, address string) (float64, error) {
	platformInfo, ok := PlatformInfos[chainId]
	if !ok {
		return -1, errors.Wrapf(errs.Unsupported, "invalid chain id: %s", chainId)
	}

	query := url.Values{
		"vs_currency": {"usd"},
		"days":        {"1"},
		"interval":    {"daily"},
	}
	path := path.Join("coins", platformInfo.Id, "contract", address, "market_chart")
	httpResp, err := c.client.Do(ctx, fasthttp.MethodGet, path, httpclient.RequestOptions{
		Query: query,
	})
	if err != nil {
		return -1, errors.Wrapf(err, "failed to get http request, path: %s", path)
	}

	status := httpResp.StatusCode()
	switch status {
	case http.StatusOK:
		resp := &coinHistoricalPriceResponse{}
		if err := httpResp.UnmarshalBody(resp); err != nil {
			return -1, errors.Wrapf(err, "failed to unmarshal response body, status %v", status)
		}

		if len(resp.Prices) == 0 {
			return -1, errors.Wrapf(errs.NotFound, "got empty prices")
		}

		latestPrice := resp.Prices[len(resp.Prices)-1]
		if len(latestPrice) != 2 {
			return -1, errors.Wrapf(errs.InternalError, "invalid price format, got: %+v", latestPrice)
		}
		return latestPrice[1], nil
	case http.StatusNotFound:
		return -1, errors.Wrapf(errs.NotFound, "not found, path: %s", path)
	default:
		return -1, errors.Wrapf(errs.InternalError, "failed to do http request, path: %s, status: %d, body: %s", path, status, string(httpResp.Body()))
	}
}
