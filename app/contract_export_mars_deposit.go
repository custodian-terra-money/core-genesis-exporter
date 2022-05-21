package app

import (
	"context"
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
	wasmtypes "github.com/terra-money/core/x/wasm/types"
)

var (
	marsMarket        = "terra19dtgj9j5j7kyf3pmejqv8vzfpxtejaypgzkz5u"
	maLunaToken       = "terra1x4rrkxx5pyuce32wsdn8ypqnpx8n27klnegv0d"
	marsLunaLiquidity = ""
	marsUSTLiquidity  = ""
	marsFields        = []string{
		//marsLunaUstField
		"terra1kztywx50wv38r58unxj9p6k3pgr2ux6w5x68md",
		// marsAncUstField
		"terra1vapq79y9cqghqny7zt72g4qukndz282uvqwtz6",
		// marsMirUstField
		"terra12dq4wmfcsnz6ycep6ek4umtuaj6luhfp256hyu",
	}
	astroportGenerator = "terra1zgrx9jjqrfye8swykfgmd6hpde60j0nszzupp9"
)

// To prevent double counting, snapshot only assign depositors what is left in the 'bank'
// borrowers are eligible for the snapshot
// Logic:
// 1. Find ownership of maTokens
// 2. Find total supply of maTokens
// 3. Find balance of assets in bank
// 4. Assign accounts with assets proportionally
func ExportMarsDepositLuna(app *TerraApp, q wasmtypes.QueryServer) (map[string]sdk.Int, error) {
	ctx := prepCtx(app)
	logger := app.Logger()

	var balances = make(map[string]sdk.Int)
	logger.Info("fetching MARS liquidity (LUNA)...")

	if err := getCW20AccountsAndBalances2(ctx, app.WasmKeeper, maLunaToken, balances); err != nil {
		return nil, err
	}
	marsMarketAddr, err := sdk.AccAddressFromBech32(marsMarket)
	if err != nil {
		return nil, err
	}
	coin := app.BankKeeper.GetBalance(sdk.UnwrapSDKContext(ctx), marsMarketAddr, "uluna")
	fmt.Printf("total amount in bank: %v\n", coin)
	totalSupply, err := getCW20TotalSupply(ctx, q, maLunaToken)
	fmt.Printf("total supply of maToken: %v\n", totalSupply)

	sum := sdk.NewInt(0)
	// balance * ER
	for address, balance := range balances {
		if balance.IsZero() {
			continue
		}
		balances[address] = balance.Mul(coin.Amount).Quo(totalSupply)
		sum = sum.Add(balances[address])
	}
	// There is rounding error here. Should we assign this fairly or ignore it? (<1000 uluna)
	fmt.Printf("%s, %s, difference: %s\n", sum, coin.Amount, coin.Amount.Sub(sum))
	return balances, nil
}

func ExportMarsDepositUST(app *TerraApp, q wasmtypes.QueryServer) (map[string]sdk.Int, error) {
	ctx := prepCtx(app)
	logger := app.Logger()

	var balances = make(map[string]sdk.Int)
	logger.Info("fetching MARS liquidity (UST)...")

	if err := getCW20AccountsAndBalances(ctx, balances, marsUSTLiquidity, q); err != nil {
		return nil, err
	}

	// get luna liquidity <> luna er
	var lunaMarketState struct {
		LiquidityIndex sdk.Dec `json:"liquidity_index"`
	}
	if err := contractQuery(ctx, q, &wasmtypes.QueryContractStoreRequest{
		ContractAddress: marsMarket,
		QueryMsg:        []byte("{\"market\": {\"asset\": {\"native\": {\"denom\": \"uusd\"}}}}"),
	}, &lunaMarketState); err != nil {
		return nil, err
	}

	// balance * ER
	for address, balance := range balances {
		balances[address] = lunaMarketState.LiquidityIndex.MulInt(balance).TruncateInt()
	}

	return balances, nil
}

// Get eventual ownership of LP tokens in the Field of Mars (leveraged yield farming) contracts
// 1. Get the LP token contract addr
// 2. List all positions recurrsively
// 3. Create a holding map with format {"lp_token_addr": {"wallet_addr": "amount"}}
func ExportFieldOfMarsLpTokens(app *TerraApp, q wasmtypes.QueryServer) (map[string]map[string]sdk.Int, error) {
	ctx := prepCtx(app)
	holdings := make(map[string]map[string]sdk.Int)
	lpTokenFieldMap := make(map[string]string)
	for _, fieldContract := range marsFields {
		err := getFieldOfMarsPositions(ctx, q, fieldContract, holdings, lpTokenFieldMap)
		if err != nil {
			app.Logger().Error(err.Error())
			return nil, err
		}
	}

	for lpToken, holdings := range holdings {
		fieldContract := lpTokenFieldMap[lpToken]
		auditAstroportLpBalances(ctx, q, astroportGenerator, lpToken, holdings, fieldContract)
	}
	return holdings, nil
}

func auditAstroportLpBalances(ctx context.Context, q wasmtypes.QueryServer, astroportGenerator string, lpToken string, holdings map[string]sdk.Int, vaultAddr string) error {
	astroportDeposits, err := getAstroportGeneratorDeposit(ctx, q, astroportGenerator, lpToken, vaultAddr)
	if err != nil {
		return err
	}
	totalHolding := sdk.NewInt(0)
	for _, balance := range holdings {
		totalHolding = totalHolding.Add(balance)
	}

	fmt.Printf("lp_token: %s, difference: %s, astroport: %s, total %s\n", lpToken, totalHolding.Sub(astroportDeposits), astroportDeposits, totalHolding)
	return nil
}

func getAstroportGeneratorDeposit(ctx context.Context, q wasmtypes.QueryServer, astroportGenerator string, lpToken string, user string) (sdk.Int, error) {
	query := fmt.Sprintf("{\"deposit\": {\"user\": \"%s\", \"lp_token\": \"%s\"}}", user, lpToken)
	var amount sdk.Int
	err := contractQuery(ctx, q, &wasmtypes.QueryContractStoreRequest{
		ContractAddress: astroportGenerator,
		QueryMsg:        []byte(query),
	}, &amount)
	if err != nil {
		return amount, err
	}
	return amount, nil
}

func getFieldOfMarsPositions(
	ctx context.Context,
	q wasmtypes.QueryServer,
	fieldContract string,
	holdings map[string]map[string]sdk.Int,
	lpTokenFieldMap map[string]string,
) error {
	var fieldConfig struct {
		PrimaryPair struct {
			LiquidityToken string `json:"liquidity_token"`
		} `json:"primary_pair"`
	}
	err := contractQuery(ctx, q, &wasmtypes.QueryContractStoreRequest{
		ContractAddress: fieldContract,
		QueryMsg:        []byte("{\"config\":{}}"),
	}, &fieldConfig)
	if err != nil {
		return err
	}
	lpTokenFieldMap[fieldConfig.PrimaryPair.LiquidityToken] = fieldContract

	var fieldState struct {
		TotalBondUnits sdk.Int `json:"total_bond_units"`
	}
	err = contractQuery(ctx, q, &wasmtypes.QueryContractStoreRequest{
		ContractAddress: fieldContract,
		QueryMsg:        []byte("{\"state\": {}}"),
	}, &fieldState)
	if err != nil {
		return err
	}

	astroportGeneratorBalance, err := getAstroportGeneratorDeposit(ctx, q, astroportGenerator, fieldConfig.PrimaryPair.LiquidityToken, fieldContract)
	if err != nil {
		return err
	}

	type Position struct {
		User     string `json:"user"`
		Position struct {
			BondUnits sdk.Int `json:"bond_units"`
		} `json:"position"`
	}

	limit := 20
	var positions []Position
	var getPositions func(string) error
	getPositions = func(lastAcc string) error {
		// fmt.Printf("last account: %s, len: %d\n", lastAcc, len(positions))
		query := fmt.Sprintf("{\"positions\":{\"limit\": %d,\"start_after\":\"%s\"}}", limit, lastAcc)
		var pagedPositions []Position
		err := contractQuery(ctx, q, &wasmtypes.QueryContractStoreRequest{
			ContractAddress: fieldContract,
			QueryMsg:        []byte(query),
		}, &pagedPositions)
		if err != nil {
			return err
		}
		positions = append(positions, pagedPositions...)
		if len(pagedPositions) < limit {
			return nil
		} else {
			return getPositions(pagedPositions[len(pagedPositions)-1].User)
		}
	}
	err = getPositions("")
	if err != nil {
		return err
	}
	fmt.Printf("number of positions: %d\n", len(positions))

	lpHoldings := make(map[string]sdk.Int)
	for _, pos := range positions {
		lpHoldings[pos.User] = pos.Position.BondUnits.Mul(astroportGeneratorBalance).Quo(fieldState.TotalBondUnits)
	}
	holdings[fieldConfig.PrimaryPair.LiquidityToken] = lpHoldings
	return nil
}