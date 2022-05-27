package native

import (
	"fmt"
	"github.com/cosmos/cosmos-sdk/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	terra "github.com/terra-money/core/app"
	"github.com/terra-money/core/app/export/util"
)

func ExportAllBondedLuna(app *terra.TerraApp) (util.SnapshotBalanceAggregateMap, error) {
	ctx := util.PrepCtx(app)
	uCtx := types.UnwrapSDKContext(ctx)

	validators := app.StakingKeeper.GetAllValidators(uCtx)
	valMap := make(map[string]stakingtypes.Validator)
	for _, v := range validators {
		valMap[v.OperatorAddress] = v
	}

	var unbondingDelegations []stakingtypes.UnbondingDelegation
	app.StakingKeeper.IterateUnbondingDelegations(uCtx, func(_ int64, ubd stakingtypes.UnbondingDelegation) (stop bool) {
		unbondingDelegations = append(unbondingDelegations, ubd)
		return false
	})

	c := 0
	snapshot := make(util.SnapshotBalanceAggregateMap)
	app.StakingKeeper.IterateAllDelegations(uCtx, func(del stakingtypes.Delegation) (stop bool) {
		c += 1
		if c%10000 == 0 {
			app.Logger().Info(fmt.Sprintf("Iterating delegations.. %d", c))
		}
		v, ok := valMap[del.ValidatorAddress]
		if !ok {
			return false
		}
		snapshot.AppendOrAddBalance(del.DelegatorAddress, util.SnapshotBalance{
			Denom:   util.DenomLUNA,
			Balance: v.TokensFromShares(del.Shares).TruncateInt(),
		})
		return false
	})

	for _, ub := range unbondingDelegations {
		for _, entry := range ub.Entries {
			snapshot.AppendOrAddBalance(ub.DelegatorAddress, util.SnapshotBalance{
				Denom:   util.DenomLUNA,
				Balance: entry.Balance,
			})
		}
	}
	return snapshot, nil
}

func ExportAllNativeBalances(app *terra.TerraApp) (util.SnapshotBalanceAggregateMap, error) {
	ctx := util.PrepCtx(app)
	snapshot := make(util.SnapshotBalanceAggregateMap)
	balances := app.BankKeeper.GetAccountsBalances(types.UnwrapSDKContext(ctx))
	for _, balance := range balances {
		for _, coin := range balance.Coins {
			if !coin.Amount.IsZero() {
				snapshot.AppendOrAddBalance(balance.Address, util.SnapshotBalance{
					Denom:   coin.Denom,
					Balance: coin.Amount,
				})
			}
		}
	}
	return snapshot, nil
}
