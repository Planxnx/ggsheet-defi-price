package arken

import (
	"context"
	"path"

	"github.com/ethereum/go-ethereum/common"
)

type Price struct {
	Price            float64 `json:"price"`
	LastUpdatedAt    int     `json:"lastUpdatedAt"`
	LastUpdatedBlock int     `json:"lastUpdatedBlock"`
}

func (a *Client) GetPrice(ctx context.Context, chainID string, address common.Address) (float64, error) {
	var price Price
	_, err := a.doRequest(ctx, path.Join("insider/v1", chainID, "tokens/price", address.String()), &price)
	if err != nil {
		return 0, err
	}
	return price.Price, nil
}
