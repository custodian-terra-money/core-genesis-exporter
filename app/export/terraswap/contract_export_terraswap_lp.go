package app

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	terra "github.com/terra-money/core/app"
	util "github.com/terra-money/core/app/export/util"
	wasmtypes "github.com/terra-money/core/x/wasm/types"
)

var (
	terraswapFactory = "terra1ulgw0td86nvs4wtpsc80thv6xelk76ut7a7apj"
	astroportFactory = ""
	loopFactory      = ""
)

// ExportTerraswapLiquidity scan all factory contracts, look for pairs that have luna or ust,
// then
func ExportTerraswapLiquidity(app *terra.TerraApp, q wasmtypes.QueryServer) (map[string]sdk.Int, error) {
	ctx := util.PrepCtx(app)
	// logger := app.Logger()

	// get all pairs from factory
	var pairsResponse struct {
		Pairs []struct {
			AssetInfos []struct {
				NativeToken struct {
					Denom string `json:"denom"`
				} `json:"native_token"`
				Token struct {
					ContractAddr string `json:"contract_addr"`
				} `json:"token"`
			} `json:"asset_infos"`
			ContractAddr   string `json:"contract_addr"`
			LiquidityToken string `json:"liquidity_token"`
		} `json:"pairs"`
	}

	if err := util.ContractQuery(ctx, q, &wasmtypes.QueryContractStoreRequest{
		ContractAddress: terraswapFactory,
		QueryMsg:        []byte("{\"pairs\":{}}"),
	}, &pairsResponse); err != nil {
		return nil, err
	}
	return nil, nil
}