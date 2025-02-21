package coingecko

import (
	"github.com/gaze-network/indexer-network/pkg/httpclient"
	"github.com/gaze-network/indexer-network/pkg/logger"
	"github.com/gaze-network/indexer-network/pkg/logger/slogx"
)

// Public/Demo Endpoint
// Rate-Limit: 10k req/month (322 req/day)
const apiURL = "https://api.coingecko.com/api/v3"

type Client struct {
	client *httpclient.Client
}

func NewClient(apiKey string) *Client {
	if apiKey == "" {
		logger.Error("empty api key", slogx.String("module", "coingecko"))
		panic("empty api key")
	}

	client, err := httpclient.New(apiURL, httpclient.Config{
		Debug: true,
		Headers: map[string]string{
			"x-cg-demo-api-key": apiKey,
		},
	})
	if err != nil {
		logger.Error("failed to create http client",
			slogx.String("module", "coingecko"),
			slogx.String("url", apiURL),
			slogx.Error(err))
		panic(err)
	}

	return &Client{
		client: client,
	}
}

func (c *Client) IsSupportedChainId(chainId string) bool {
	_, ok := PlatformInfos[chainId]
	return ok
}

// NetworkId -> Platform Info
// Ref: https://docs.coingecko.com/v3.0.1/reference/asset-platforms-list
var PlatformInfos = map[string]PlatformInfo{
	"1": {
		Id:      "ethereum",
		ChainId: "1",
		Name:    "Ethereum",
	},
	"42161": {
		Id:      "arbitrum-one",
		ChainId: "42161",
		Name:    "Arbitrum One",
	},
	"56": {
		Id:      "binance-smart-chain",
		ChainId: "56",
		Name:    "BNB Smart Chain",
	},
	"solana": {
		Id:      "solana",
		ChainId: "solana",
		Name:    "Solana",
	},
}

type PlatformInfo struct {
	Id      string // coingecko id
	ChainId string
	Name    string
}
