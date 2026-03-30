package ethereum

import (
	"context"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
)

// Transfer defines the transfer event type
type Transfer struct {
	From     common.Address
	To       common.Address
	Amount   *big.Int
	TxHash   common.Hash
	LogIndex uint
	BlockNum uint64
}

// TokenClient wraps the Ethereum client and token contract interaction
type TokenClient struct {
	client       *ethclient.Client
	contractAddr common.Address
}

// NewTokenClient creates a new TokenClient instance
func NewTokenClient(client *ethclient.Client, contractAddr common.Address) (*TokenClient, error) {
	return &TokenClient{
		client:       client,
		contractAddr: contractAddr,
	}, nil
}

// GetBalance returns the token balance for a given address
func (tc *TokenClient) GetBalance(ctx context.Context, address common.Address) (*big.Int, error) {
	// This would use the actual contract binding in a real implementation
	// For now, we'll leave this as a placeholder
	return big.NewInt(0), nil
}

// GetTotalSupply returns the total token supply
func (tc *TokenClient) GetTotalSupply(ctx context.Context) (*big.Int, error) {
	// This would use the actual contract binding in a real implementation
	// For now, we'll leave this as a placeholder
	return big.NewInt(0), nil
}

// GetTransferEvents fetches transfer events from the blockchain
func (tc *TokenClient) GetTransferEvents(ctx context.Context, fromBlock, toBlock *big.Int) ([]*Transfer, error) {
	// Transfer event signature: Transfer(address,address,uint256)
	transferEventSig := common.HexToHash("0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef")

	// Filter logs for transfer events
	logs, err := tc.client.FilterLogs(ctx, ethereum.FilterQuery{
		FromBlock: fromBlock,
		ToBlock:   toBlock,
		Addresses: []common.Address{tc.contractAddr},
		Topics:    [][]common.Hash{{transferEventSig}},
	})
	if err != nil {
		return nil, err
	}

	// Parse the logs into Transfer events
	transfers := make([]*Transfer, 0, len(logs))
	for _, log := range logs {
		if len(log.Topics) < 3 {
			continue
		}

		transfer := &Transfer{
			From:     common.BytesToAddress(log.Topics[1].Bytes()),
			To:       common.BytesToAddress(log.Topics[2].Bytes()),
			Amount:   new(big.Int).SetBytes(log.Data),
			TxHash:   log.TxHash,
			LogIndex: log.Index,
			BlockNum: log.BlockNumber,
		}
		transfers = append(transfers, transfer)
	}

	return transfers, nil
}

// GetContractDeploymentBlock finds the block number where the contract was deployed
func (tc *TokenClient) GetContractDeploymentBlock(ctx context.Context) (*big.Int, error) {
	// Get the latest block number
	latestBlock, err := tc.client.BlockNumber(ctx)
	if err != nil {
		return nil, err
	}

	// Binary search for the deployment block
	low := big.NewInt(0)
	high := big.NewInt(int64(latestBlock))
	deploymentBlock := big.NewInt(0)

	for low.Cmp(high) <= 0 {
		mid := new(big.Int).Add(low, high)
		mid.Div(mid, big.NewInt(2))

		// Check if contract exists at this block
		code, err := tc.client.CodeAt(ctx, tc.contractAddr, mid)
		if err != nil {
			return nil, err
		}

		if len(code) > 0 {
			// Contract exists at this block, try to find an earlier block
			deploymentBlock.Set(mid)
			high.Sub(mid, big.NewInt(1))
		} else {
			// Contract doesn't exist at this block, try later blocks
			low.Add(mid, big.NewInt(1))
		}
	}

	// If we found a deployment block, return it, otherwise return error
	if deploymentBlock.Cmp(big.NewInt(0)) > 0 {
		return deploymentBlock, nil
	}

	return nil, fmt.Errorf("contract not found at any block")
}
